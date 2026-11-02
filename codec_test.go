package textcodecs

import (
	"errors"
	"testing"
)

func testCodec() *SingleByte {
	c := &SingleByte{EncodeTable: map[rune]byte{'A': 0x41, '€': 0x80, 0: 0}}
	c.DecodeTable[0x41] = 'A' + 1
	c.DecodeTable[0x80] = '€' + 1
	c.DecodeTable[0] = 1
	return c
}

func TestConversionAndBoundaries(t *testing.T) {
	c := testCodec()
	dst := make([]byte, 8)
	n, used, err := c.Decode(dst, []byte{'A', 0x80, 0})
	if err != nil || used != 3 || string(dst[:n]) != "A€\x00" {
		t.Fatalf("decode: %d %d %v %q", n, used, err, dst[:n])
	}
	n, used, err = c.Encode(dst, []byte("A€\x00"), true)
	if err != nil || used != 5 || string(dst[:n]) != "A\x80\x00" {
		t.Fatalf("encode: %d %d %v %q", n, used, err, dst[:n])
	}
	n, used, err = c.Decode(dst[:3], []byte{'A', 0x80})
	if n != 1 || used != 1 || !errors.Is(err, ErrShortDst) {
		t.Fatalf("short dst decode: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(dst[:1], []byte("A€"), true)
	if n != 1 || used != 1 || !errors.Is(err, ErrShortDst) {
		t.Fatalf("short dst encode: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(dst, []byte{'A', 0xe2, 0x82}, false)
	if n != 1 || used != 1 || !errors.Is(err, ErrShortSrc) {
		t.Fatalf("short src: %d %d %v", n, used, err)
	}
	_, _, err = c.Encode(dst, []byte{0xe2, 0x82}, true)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("truncated EOF: %v", err)
	}
	_, _, err = c.Encode(dst, []byte{0xff}, false)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid UTF-8: %v", err)
	}
	_, used, err = c.Decode(dst, []byte{0xff})
	if used != 0 || !errors.Is(err, ErrUnmapped) {
		t.Fatalf("unmapped byte: %d %v", used, err)
	}
	_, used, err = c.Encode(dst, []byte("☃"), true)
	if used != 0 || !errors.Is(err, ErrUnmapped) {
		t.Fatalf("unmapped rune: %d %v", used, err)
	}
}
