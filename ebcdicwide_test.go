package textcodecs

import (
	"bytes"
	"testing"
)

func TestWideShiftStreaming(t *testing.T) {
	c, ok := OpenWideShift("ibm-1390_P110-2003")
	if !ok {
		t.Fatal("missing converter")
	}
	src := []byte("Aæ̀𠀋A")
	want := []byte{0xc1, 0x0e, 0xec, 0xc3, 0xb3, 0x42, 0x0f, 0xc1}
	dst := make([]byte, 64)
	for split := 0; split <= len(src); split++ {
		c.Reset()
		n1, used, err := c.Encode(dst, src[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("encode split %d: %v", split, err)
		}
		n2, used2, err := c.Encode(dst[n1:], src[used:], true)
		if err != nil || used+used2 != len(src) || !bytes.Equal(dst[:n1+n2], want) {
			t.Fatalf("encode split %d: %x %d %d %v", split, dst[:n1+n2], used, used2, err)
		}
	}
	for split := 0; split <= len(want); split++ {
		c.Reset()
		n1, used, err := c.Decode(dst, want[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("decode split %d: %v", split, err)
		}
		n2, used2, err := c.Decode(dst[n1:], want[used:], true)
		if err != nil || used+used2 != len(want) || !bytes.Equal(dst[:n1+n2], src) {
			t.Fatalf("decode split %d: %q %d %d %v", split, dst[:n1+n2], used, used2, err)
		}
	}
	c.Reset()
	n, used, err := c.Encode(dst, []byte("æ"), false)
	if n != 0 || used != 0 || err != ErrShortSrc {
		t.Fatalf("pair lookahead: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(dst, []byte("æ"), true)
	if err != nil || used != len("æ") || !bytes.Equal(dst[:n], []byte{0x0e, 0xd6, 0x7b, 0x0f}) {
		t.Fatalf("single fallback %x %d %v", dst[:n], used, err)
	}
	c.Reset()
	n, used, err = c.Decode(dst[:3], []byte{0x0e, 0xec, 0xc3}, true)
	if n != 0 || used != 1 || err != ErrShortDst {
		t.Fatalf("atomic pair: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(dst, []byte{0xec, 0xc3}, true)
	if err != nil || used != 2 || string(dst[:n]) != "æ̀" {
		t.Fatalf("pair retry %q %d %v", dst[:n], used, err)
	}
}

func TestWideDBCS(t *testing.T) {
	c, ok := OpenWideShift("ibm-16684_P110-2003")
	if !ok {
		t.Fatal("missing converter")
	}
	dst := make([]byte, 16)
	n, used, err := c.Encode(dst, []byte("æ̀"), true)
	if err != nil || used != len("æ̀") || !bytes.Equal(dst[:n], []byte{0xec, 0xc3}) {
		t.Fatalf("encode %x %d %v", dst[:n], used, err)
	}
	n, used, err = c.Decode(dst, []byte{0xec, 0xc3}, true)
	if err != nil || used != 2 || string(dst[:n]) != "æ̀" {
		t.Fatalf("decode %q %d %v", dst[:n], used, err)
	}
}

func TestWideShiftAllSequenceSplits(t *testing.T) {
	for _, name := range WideShiftNames() {
		c, ok := OpenWideShift(name)
		if !ok {
			t.Fatal(name)
		}
		for i := 0; i < c.entry.pairCount; i++ {
			p := c.entry.encodePairs + i*10
			src := []byte(string(rune(ew32(p))) + string(rune(ew32(p+4))))
			want := make([]byte, 16)
			c.Reset()
			n, used, err := c.Encode(want, src, true)
			if err != nil || used != len(src) {
				t.Fatalf("%s pair %x: %d %v", name, src, used, err)
			}
			want = want[:n]
			for split := 0; split <= len(src); split++ {
				dst := make([]byte, 16)
				c.Reset()
				n1, used, err := c.Encode(dst, src[:split], false)
				if err != nil && err != ErrShortSrc {
					t.Fatalf("%s split %d: %v", name, split, err)
				}
				n2, used2, err := c.Encode(dst[n1:], src[used:], true)
				if err != nil || used+used2 != len(src) || !bytes.Equal(dst[:n1+n2], want) {
					t.Fatalf("%s split %d: %x want %x, %d %d %v", name, split, dst[:n1+n2], want, used, used2, err)
				}
			}
			for split := 0; split <= len(want); split++ {
				dst := make([]byte, 16)
				c.Reset()
				n1, used, err := c.Decode(dst, want[:split], false)
				if err != nil && err != ErrShortSrc {
					t.Fatalf("%s decode split %d: %v", name, split, err)
				}
				n2, used2, err := c.Decode(dst[n1:], want[used:], true)
				if err != nil || used+used2 != len(want) || !bytes.Equal(dst[:n1+n2], src) {
					t.Fatalf("%s decode split %d: %x want %x, %d %d %v", name, split, dst[:n1+n2], src, used, used2, err)
				}
			}
		}
	}
}
