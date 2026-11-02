package textcodecs

import (
	"bytes"
	"testing"
)

func TestStatefulStreaming(t *testing.T) {
	c, ok := OpenStateful("IBM-930_p120-1999")
	if !ok {
		t.Fatal("missing converter")
	}
	src := []byte("AあいA")
	encoded := []byte{0xc1, 0x0e, 0x44, 0x81, 0x44, 0x82, 0x0f, 0xc1}
	dst := make([]byte, 64)
	for split := 0; split <= len(src); split++ {
		c.Reset()
		n1, used, err := c.Encode(dst, src[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("encode split %d: %v", split, err)
		}
		n2, used2, err := c.Encode(dst[n1:], src[used:], true)
		if err != nil || used+used2 != len(src) || !bytes.Equal(dst[:n1+n2], encoded) {
			t.Fatalf("encode split %d: %x %d %d %v", split, dst[:n1+n2], used, used2, err)
		}
	}
	for split := 0; split <= len(encoded); split++ {
		c.Reset()
		n1, used, err := c.Decode(dst, encoded[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("decode split %d: %v", split, err)
		}
		n2, used2, err := c.Decode(dst[n1:], encoded[used:], true)
		if err != nil || used+used2 != len(encoded) || !bytes.Equal(dst[:n1+n2], src) {
			t.Fatalf("decode split %d: %q %d %d %v", split, dst[:n1+n2], used, used2, err)
		}
	}
	c.Reset()
	n, used, err := c.Encode(dst[:3], []byte("あ"), true)
	if n != 3 || used != len("あ") || err != ErrShortDst || !bytes.Equal(dst[:n], []byte{0x0e, 0x44, 0x81}) {
		t.Fatalf("final shift short: %x %d %v", dst[:n], used, err)
	}
	n, used, err = c.Encode(dst, nil, true)
	if n != 1 || used != 0 || err != nil || dst[0] != 0x0f {
		t.Fatalf("final shift retry: %x %d %v", dst[:n], used, err)
	}
	c.Reset()
	n, used, err = c.Decode(dst, []byte{0x0e, 0x44}, false)
	if n != 0 || used != 1 || err != ErrShortSrc {
		t.Fatalf("decode partial: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(dst, []byte{0x44, 0x81}, true)
	if n != len("あ") || used != 2 || err != nil || string(dst[:n]) != "あ" {
		t.Fatalf("decode retry: %q %d %v", dst[:n], used, err)
	}
	c.Reset()
	n, used, err = c.Encode(dst[:2], []byte("あ"), false)
	if n != 0 || used != 0 || err != ErrShortDst {
		t.Fatalf("atomic shift: %d %d %v", n, used, err)
	}
}

func TestCompoundShiftState(t *testing.T) {
	c, ok := OpenStateful("icu-internal-25546")
	if !ok {
		t.Fatal("missing converter")
	}
	out := make([]byte, 32)
	n, used, err := c.Encode(out, []byte("Aあ"), true)
	want := []byte{'A', 0x0e, 0x2a, 0x22, 0x0f}
	if err != nil || used != len("Aあ") || !bytes.Equal(out[:n], want) {
		t.Fatalf("encode: %x %d %v", out[:n], used, err)
	}
	n, used, err = c.Decode(out, want, true)
	if err != nil || used != len(want) || string(out[:n]) != "Aあ" {
		t.Fatalf("decode: %q %d %v", out[:n], used, err)
	}
	n, used, err = c.Decode(out, []byte{0x80}, true)
	if n != 0 || used != 0 || err != ErrInvalid {
		t.Fatalf("illegal byte: %d %d %v", n, used, err)
	}
}
