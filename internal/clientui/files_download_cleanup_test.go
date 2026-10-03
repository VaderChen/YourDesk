package clientui

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadCleanupRetriesOnlyExplicitCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	control := newFileDownloadControl(parent, nil)
	control.stop()
	var attempts atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- cleanupFileDownload(control, func() error {
			if attempts.Add(1) < 3 {
				return os.ErrPermission
			}
			return nil
		})
	}()
	for want := int32(1); want <= 2; want++ {
		waitControlledState(t, control, "cancelFailed")
		if attempts.Load() != want {
			t.Fatalf("cleanup retried without cancellation: %d", attempts.Load())
		}
		if _, err := control.pause(); err == nil || control.resume() == nil {
			t.Fatal("cleanup failure must not pause or resume file reads")
		}
		control.interrupt(nil)
		if state := control.snapshot().State; state != "cancelFailed" {
			t.Fatalf("reconnect changed cleanup state: %q", state)
		}
		select {
		case err := <-done:
			t.Fatalf("unconfirmed cleanup finished: %v", err)
		default:
		}
		control.stop()
	}
	select {
	case err := <-done:
		if err != nil || attempts.Load() != 3 {
			t.Fatalf("cleanup retry failed: %v, attempts=%d", err, attempts.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("explicit cleanup retry did not finish")
	}
}

func TestDownloadCleanupDoesNotLoseCancellationDuringRemoval(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	control := newFileDownloadControl(parent, nil)
	control.stop()
	started, release := make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- cleanupFileDownload(control, func() error {
			if attempts.Add(1) == 1 {
				close(started)
				<-release
				return os.ErrPermission
			}
			return nil
		})
	}()
	<-started
	control.stop()
	close(release)
	select {
	case err := <-done:
		if err != nil || attempts.Load() != 2 {
			t.Fatalf("pending cancellation was lost: %v, %d", err, attempts.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup waited despite an explicit retry during removal")
	}
}

func TestDownloadCleanupParentExitReturnsFailure(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	control := newFileDownloadControl(parent, nil)
	done := make(chan error, 1)
	go func() { done <- cleanupFileDownload(control, func() error { return os.ErrPermission }) }()
	waitControlledState(t, control, "cancelFailed")
	cancelParent()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("parent exit concealed cleanup failure: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parent exit left cleanup worker blocked")
	}
}

func TestDownloadCleanupAlreadyRemovedIsComplete(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	control := newFileDownloadControl(parent, nil)
	control.stop()
	// Another process can remove the path after the ownership Lstat succeeds.
	if err := cleanupFileDownload(control, func() error { return os.ErrNotExist }); err != nil {
		t.Fatalf("already removed partial required another cancellation: %v", err)
	}
	if state := control.snapshot().State; state != "cancelled" {
		t.Fatalf("already removed partial reported cleanup failure: %q", state)
	}
}

func TestDownloadIdentityPinSurvivesWriterClose(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	writer, err := root.OpenFile("partial", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	original, err := writer.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pin, err := duplicateDownloadFile(writer)
	if err != nil {
		t.Fatal(err)
	}
	defer pin.Close()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove("partial"); err != nil {
		t.Fatalf("pin prevents cleanup: %v", err)
	}
	if err := root.WriteFile("partial", []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	pinned, err := pin.Stat()
	if err != nil || !os.SameFile(original, pinned) {
		t.Fatal("pin lost original file identity", err)
	}
	replacement, err := root.Lstat("partial")
	if err != nil || os.SameFile(pinned, replacement) {
		t.Fatal("pin was replaced along with the path", err)
	}
	if other, err := duplicateDownloadFile(writer); err == nil {
		other.Close()
		t.Fatal("duplicated a closed writer")
	}
}

func preventDownloadRemoval(t *testing.T, directory string) {
	t.Helper()
	probe := filepath.Join(directory, "permission-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0500); err != nil {
		t.Skip("filesystem cannot remove directory write permission", err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0700) })
	if err := os.Remove(probe); err == nil {
		t.Skip("directory permissions do not prevent removal on this platform")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
}

func TestDownloadCancelCleanupFailureRetainsOwnership(t *testing.T) {
	for _, action := range []string{"retry", "replacement", "parent-exit", "stat-denied"} {
		t.Run(action, func(t *testing.T) {
			client := controlledDownloadFixture(t, bytes.Repeat([]byte{7}, 9000), func(w http.ResponseWriter, _ *http.Request, in controlledDownloadRequest) bool {
				if in.Action == "read" && in.Params.Offset == 4096 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return true
				}
				return false
			})
			parent, cancelParent := context.WithCancel(context.Background())
			control := newFileDownloadControl(parent, nil)
			done := make(chan controlledDownloadResult, 1)
			go func() {
				out, err := client.prepareControlled(control, "fixture.txt")
				done <- controlledDownloadResult{out, err}
				close(done)
			}()
			t.Cleanup(func() {
				_ = os.Chmod(client.downloads, 0700)
				cancelParent()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("download cleanup worker did not stop")
				}
			})
			waitControlledState(t, control, "interrupted")
			partials, err := downloadPartials(client.downloads)
			if err != nil || len(partials) != 1 {
				t.Fatalf("missing partial: %v, %v", partials, err)
			}
			partial := partials[0]
			preventDownloadRemoval(t, client.downloads)
			if action == "stat-denied" {
				if err := os.Chmod(client.downloads, 0); err != nil {
					t.Fatal(err)
				}
			}
			control.stop()
			progress := waitControlledState(t, control, "cancelFailed")
			if progress.Received != 4096 || progress.Speed != 0 {
				t.Fatalf("cleanup failure lost progress: %+v", progress)
			}
			select {
			case result := <-done:
				t.Fatalf("cleanup failure falsely settled preparation: %+v", result)
			default:
			}
			if action == "parent-exit" {
				cancelParent()
				result := awaitControlledResult(t, done)
				if result.err == nil || !strings.Contains(result.err.Error(), "無法清理下載暫存") || strings.Contains(result.err.Error(), client.downloads) {
					t.Fatalf("shutdown did not report sanitized cleanup failure: %v", result.err)
				}
				return
			}
			if err := os.Chmod(client.downloads, 0700); err != nil {
				t.Fatal(err)
			}
			if action == "replacement" {
				if err := os.Rename(partial, partial+".kept"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(partial, []byte("external replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			control.stop()
			if result := awaitControlledResult(t, done); !errors.Is(result.err, context.Canceled) {
				t.Fatalf("cancel result: %+v", result)
			}
			if action == "replacement" {
				if data, err := os.ReadFile(partial); err != nil || string(data) != "external replacement" {
					t.Fatal("retry removed external replacement", err)
				}
			} else if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("retry did not remove own partial: %v", err)
			}
		})
	}
}
