package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
	"yourdesk/internal/security"
)

func TestPasswordPipeSmoke(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		t.Run("line-ending", func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stdin
			os.Stdin = r
			defer func() { os.Stdin = previous; r.Close() }()
			password := "test-密碼-123456 "
			payload, _ := json.Marshal(map[string]string{"secret": password})
			if _, err = w.Write(append(payload, []byte(ending)...)); err != nil {
				t.Fatal(err)
			}
			w.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			input := newPasswordInput()
			got, err := input.read(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := security.DecodeSecret(password)
			if !bytes.Equal(got, expected) {
				t.Fatal("password changed while passing through stdin")
			}
			// Wait for the scanner to exit before restoring global stdin.
			select {
			case _, open := <-input.values:
				if open {
					t.Fatal("unexpected second input")
				}
			case <-ctx.Done():
				t.Fatal("scanner did not finish")
			}
		})
	}
}
