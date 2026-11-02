package textcodecs

import (
	"bytes"
	"testing"
)

func TestGSMExtensionStreaming(t *testing.T) {
	src := []byte("A€[]")
	want := []byte{'A', 0x1b, 0x65, 0x1b, 0x3c, 0x1b, 0x3e}
	dst := make([]byte, 32)
	n, used, err := GSM.Encode(dst, src, true)
	if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
		t.Fatalf("encode %x %d %v", dst[:n], used, err)
	}
	for split := 0; split <= len(want); split++ {
		n1, used, err := GSM.Decode(dst, want[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("split %d: %v", split, err)
		}
		n2, used2, err := GSM.Decode(dst[n1:], want[used:], true)
		if err != nil || used+used2 != len(want) || !bytes.Equal(dst[:n1+n2], src) {
			t.Fatalf("split %d: %q %d %d %v", split, dst[:n1+n2], used, used2, err)
		}
	}
	for _, tc := range []struct {
		input []byte
		want  string
	}{{[]byte{0x1b}, " "}, {[]byte{0x1b, 0x41}, " A"}, {[]byte{0x1b, 0x1b}, " "}} {
		n, used, err := GSM.Decode(dst, tc.input, true)
		if err != nil || used != len(tc.input) || string(dst[:n]) != tc.want {
			t.Fatalf("fallback %x: %q %d %v", tc.input, dst[:n], used, err)
		}
	}
	n, used, err = GSM.Decode(dst, []byte{0x1b}, false)
	if n != 0 || used != 0 || err != ErrShortSrc {
		t.Fatalf("short input: %d %d %v", n, used, err)
	}
	n, used, err = GSM.Encode(dst[:1], []byte("€"), true)
	if n != 0 || used != 0 || err != ErrShortDst {
		t.Fatalf("short output: %d %d %v", n, used, err)
	}
}
