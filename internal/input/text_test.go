package input

import (
	"reflect"
	"strings"
	"testing"
)

type textRecorder struct {
	Controller
	values []string
}

func (r *textRecorder) Text(text string) error { r.values = append(r.values, text); return nil }

func TestCommittedTextValidationAndDispatch(t *testing.T) {
	r := &textRecorder{}
	for _, text := range []string{"中文", "a😀\n", strings.Repeat("x", MaxTextBytes)} {
		if err := SendText(r, text); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.values) != 3 {
		t.Fatalf("got %d text calls", len(r.values))
	}
	for _, text := range []string{strings.Repeat("x", MaxTextBytes+1), string([]byte{0xff})} {
		if err := SendText(r, text); err == nil {
			t.Fatal("accepted invalid text")
		}
	}
	if err := SendText(r, ""); err != nil {
		t.Fatal(err)
	}
	if len(r.values) != 3 {
		t.Fatal("invalid/empty text was dispatched")
	}
	if err := SendText(nil, "中文"); err == nil {
		t.Fatal("accepted controller without Unicode support")
	}
}

func TestTextUTF16PreservesChineseAndSurrogatePairs(t *testing.T) {
	got, err := textUTF16("中😀文")
	want := []uint16{0x4e2d, 0xd83d, 0xde00, 0x6587}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %x, %v", got, err)
	}
	if _, err := textUTF16(strings.Repeat("中", MaxTextBytes/3+1)); err == nil {
		t.Fatal("byte limit used character count")
	}
}
