//go:build !winpe

package peertransport

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestTailcatLiveSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_TAILCAT_SMOKE") != "1" {
		t.Skip("set YOURDESK_TAILCAT_SMOKE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	host, err := Open(ctx, Tailcat, true, "")
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	viewer, err := Open(ctx, Tailcat, false, host.Offer())
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	for _, pair := range [][2]Link{{viewer, host}, {host, viewer}} {
		for _, size := range []int{64, 1400, 60000} {
			payload := bytes.Repeat([]byte{0x35}, size)
			pair[0].SetWriteDeadline(time.Now().Add(8 * time.Second))
			pair[1].SetReadDeadline(time.Now().Add(8 * time.Second))
			if _, err = pair[0].WriteTo(payload, pair[1].LocalAddr()); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 65536)
			n, _, e := pair[1].ReadFrom(buf)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(buf[:n], payload) {
				t.Fatal("payload mismatch")
			}
		}
	}
	t.Log("Tailcat 雙向封包與大型封包分片／重組成功")
}
