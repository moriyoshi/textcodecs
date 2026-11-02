package textcodecs

import (
	"bytes"
	"math/bits"
	"testing"
	"unicode/utf8"
)

// referenceEncode deliberately uses the standard UTF-8 decoder and does not
// share the optimized parsing or ASCII-copy paths.
func referenceEncode(dst, src []byte, eof bool) (nd, ns int, err error) {
	for ns < len(src) {
		if !eof && !utf8.FullRune(src[ns:]) {
			return nd, ns, ErrShortSrc
		}
		r, n := utf8.DecodeRune(src[ns:])
		if r == utf8.RuneError && n == 1 {
			return nd, ns, ErrInvalid
		}
		var v uint32
		if r < 65536 {
			e := testCP932.entry
			block := mbcs16(e.encodeIndex + (int(r)>>6)*2)
			if block != 0 {
				block--
				bit := uint(r) & 63
				mask := mbcs64(mbcsSharedMask + int(block)*8)
				if mask&(uint64(1)<<bit) != 0 {
					index := int(mbcs32(mbcsSharedOffset+int(block)*4)) + bits.OnesCount64(mask&((uint64(1)<<bit)-1))
					v = uint32(mbcs16(mbcsSharedEncode + index*2))
				}
			}
		}
		if v == 0 {
			if icuDefaultIgnorable(r) {
				ns += n
				continue
			}
			return nd, ns, ErrUnmapped
		}
		v--
		need := 1
		if v > 255 {
			need = 2
		}
		if len(dst)-nd < need {
			return nd, ns, ErrShortDst
		}
		if need == 2 {
			dst[nd] = byte(v >> 8)
			dst[nd+1] = byte(v)
		} else {
			dst[nd] = byte(v)
		}
		nd += need
		ns += n
	}
	return nd, ns, nil
}

func FuzzCP932Encode(f *testing.F) {
	for _, b := range [][]byte{[]byte("hello 日本語"), {0xe0, 0x80, 0x80}, {0xed, 0xa0, 0x80}, {0xe3, 0x81}, {0xff}, []byte("\u200d")} {
		f.Add(b, uint8(4), false)
	}
	f.Fuzz(func(t *testing.T, src []byte, size uint8, eof bool) {
		if len(src) > 4096 {
			t.Skip()
		}
		a, b := make([]byte, int(size)), make([]byte, int(size))
		an, as, ae := testCP932.Encode(a, src, eof)
		bn, bs, be := referenceEncode(b, src, eof)
		if an != bn || as != bs || ae != be || !bytes.Equal(a[:an], b[:bn]) {
			t.Fatalf("src=%x size=%d eof=%v got=%x,%d,%v want=%x,%d,%v", src, size, eof, a[:an], as, ae, b[:bn], bs, be)
		}
	})
}

func TestCP932ThreeByteUTF8Prefixes(t *testing.T) {
	a, b := make([]byte, 8), make([]byte, 8)
	input := []byte{0, 0, 0}
	for first := 0xe0; first < 0xf0; first++ {
		input[0] = byte(first)
		for second := 0; second < 256; second++ {
			input[1] = byte(second)
			for third := 0; third < 256; third++ {
				input[2] = byte(third)
				an, as, ae := testCP932.Encode(a, input, true)
				bn, bs, be := referenceEncode(b, input, true)
				if an != bn || as != bs || ae != be || !bytes.Equal(a[:an], b[:bn]) {
					t.Fatalf("%x: got %x,%d,%v want %x,%d,%v", input, a[:an], as, ae, b[:bn], bs, be)
				}
			}
		}
	}
}
