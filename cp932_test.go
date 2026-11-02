package textcodecs

import (
	"bytes"
	"errors"
	"testing"
	"unicode/utf8"
)

// testCP932 exercises the shared engine with ICU's CP932 mapping.
var testCP932 = func() MultibyteCodec {
	c, ok := LookupMultibyte("ibm-943_P15A-2003")
	if !ok {
		panic("CP932 mapping missing")
	}
	return c
}()

func TestCP932PrivateUseBoundaries(t *testing.T) {
	cases := []struct {
		input []byte
		want  string
	}{
		{[]byte{0xf0, 0x40}, "\ue000"},
		{[]byte{0xf0, 0x7e}, "\ue03e"},
		{[]byte{0xf0, 0x80}, "\ue03f"},
		{[]byte{0xf9, 0xfc}, "\ue757"},
	}
	dst := make([]byte, 8)
	for _, tc := range cases {
		n, used, err := testCP932.Decode(dst, tc.input, true)
		if err != nil || used != len(tc.input) || string(dst[:n]) != tc.want {
			t.Fatalf("decode %x: %x %d %v", tc.input, dst[:n], used, err)
		}
		n, used, err = testCP932.Encode(dst, []byte(tc.want), true)
		if err != nil || used != len(tc.want) || !bytes.Equal(dst[:n], tc.input) {
			t.Fatalf("encode %q: %x %d %v", tc.want, dst[:n], used, err)
		}
	}
}

func TestCP932Streaming(t *testing.T) {
	src := []byte("Hello 日本語 ｱｲｳ\x1a\x1c\x7f")
	encoded := make([]byte, len(src)*2)
	n, _, err := testCP932.Encode(encoded, src, true)
	if err != nil {
		t.Fatal(err)
	}
	encoded = encoded[:n]
	for split := 0; split <= len(encoded); split++ {
		dst := make([]byte, len(src)*2)
		n1, consumed, err := testCP932.Decode(dst, encoded[:split], false)
		if err != nil && !errors.Is(err, ErrShortSrc) {
			t.Fatalf("split %d: %v", split, err)
		}
		n2, consumed2, err := testCP932.Decode(dst[n1:], encoded[consumed:], true)
		if err != nil || consumed+consumed2 != len(encoded) || !bytes.Equal(dst[:n1+n2], src) {
			t.Fatalf("split %d: %d %d %v", split, consumed, consumed2, err)
		}
	}
	for split := 0; split <= len(src); split++ {
		dst := make([]byte, len(encoded)*2)
		n1, consumed, err := testCP932.Encode(dst, src[:split], false)
		if err != nil && !errors.Is(err, ErrShortSrc) {
			t.Fatalf("split %d: %v", split, err)
		}
		n2, consumed2, err := testCP932.Encode(dst[n1:], src[consumed:], true)
		if err != nil || consumed+consumed2 != len(src) || !bytes.Equal(dst[:n1+n2], encoded) {
			t.Fatalf("split %d: %d %d %v", split, consumed, consumed2, err)
		}
	}
}

func TestCP932Errors(t *testing.T) {
	dst := make([]byte, 8)
	cases := []struct {
		src  []byte
		eof  bool
		want error
	}{
		{[]byte{0x81}, false, ErrShortSrc}, {[]byte{0x81}, true, ErrInvalid}, {[]byte{0x81, 0x7f}, true, ErrInvalid},
	}
	for _, c := range cases {
		_, n, err := testCP932.Decode(dst, c.src, c.eof)
		if n != 0 || !errors.Is(err, c.want) {
			t.Fatalf("decode %x: %d %v", c.src, n, err)
		}
	}
	_, n, err := testCP932.Decode(dst[:2], []byte{0x82, 0xa0}, true)
	if n != 0 || !errors.Is(err, ErrShortDst) {
		t.Fatalf("short decode: %d %v", n, err)
	}
	_, n, err = testCP932.Encode(dst[:1], []byte("あ"), true)
	if n != 0 || !errors.Is(err, ErrShortDst) {
		t.Fatalf("short encode: %d %v", n, err)
	}
	_, n, err = testCP932.Encode(dst, []byte{0xff}, true)
	if n != 0 || !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid UTF-8: %d %v", n, err)
	}
	_, n, err = testCP932.Encode(dst, []byte("😀"), true)
	if n != 0 || !errors.Is(err, ErrUnmapped) {
		t.Fatalf("supplementary: %d %v", n, err)
	}
	if !utf8.ValidString("日本語") {
		t.Fatal("invalid test")
	}
}

func TestCP932ICUIgnorables(t *testing.T) {
	for _, s := range []string{"\u00ad", "\u200d", "\ufe0f", "\U000e0100"} {
		n, used, err := testCP932.Encode(nil, []byte(s), true)
		if n != 0 || used != len(s) || err != nil {
			t.Fatalf("encode %U: %d %d %v", []rune(s)[0], n, used, err)
		}
	}
}

func BenchmarkCP932(b *testing.B) {
	for _, s := range []struct{ name, text string }{{"ASCII", "The quick brown fox jumps over the lazy dog. "}, {"Japanese", "日本語の文章を変換します。半角ｶﾅもあります。"}} {
		src := bytes.Repeat([]byte(s.text), 256)
		dst := make([]byte, len(src)*2)
		b.Run(s.name+"/encode", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := testCP932.Encode(dst, src, true)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		n, _, err := testCP932.Encode(dst, src, true)
		if err != nil {
			b.Fatal(err)
		}
		encoded := append([]byte(nil), dst[:n]...)
		b.Run(s.name+"/decode", func(b *testing.B) {
			b.SetBytes(int64(len(encoded)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := testCP932.Decode(dst, encoded, true)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestCopyASCIIMatchesScalar(t *testing.T) {
	for length := 0; length <= 100; length++ {
		for pos := 0; pos < length; pos++ {
			for c := 0; c < 256; c++ {
				src := bytes.Repeat([]byte{'A'}, length)
				src[pos] = byte(c)
				for _, size := range []int{0, length / 2, length} {
					got, want := make([]byte, size), make([]byte, size)
					n := copyASCII(got, src)
					m := copyASCIIScalar(want, src)
					if n != m || !bytes.Equal(got, want) {
						t.Fatalf("len=%d pos=%d byte=%02x dst=%d got=%d want=%d", length, pos, c, size, n, m)
					}
				}
			}
		}
	}
}

func TestCP932Replacement(t *testing.T) {
	dst := make([]byte, 32)
	unassigned := []byte{0x81, 0xad}
	if _, _, err := testCP932.Decode(dst, unassigned, true); err != ErrUnmapped {
		t.Fatalf("expected unassigned byte pair: %v", err)
	}
	n, s, err := testCP932.DecodeReplace(dst, unassigned, true)
	if err != nil || s != 2 || string(dst[:n]) != "\ufffd" {
		t.Fatalf("unassigned %x: %q %d %v", unassigned, dst[:n], s, err)
	}
	n, s, err = testCP932.DecodeReplace(dst, []byte{'A', 0x81, 0x20, 0x81}, true)
	if err != nil || s != 4 || string(dst[:n]) != "A\ufffd \ufffd" {
		t.Fatalf("decode: %q %d %v", dst[:n], s, err)
	}
	n, s, err = testCP932.DecodeReplace(dst[:3], []byte{'A', 0x81, 0x20}, true)
	if n != 1 || s != 1 || err != ErrShortDst {
		t.Fatalf("short replacement: %d %d %v", n, s, err)
	}
	n, s, err = testCP932.DecodeReplace(dst, []byte{0x81}, false)
	if n != 0 || s != 0 || err != ErrShortSrc {
		t.Fatalf("incomplete: %d %d %v", n, s, err)
	}
	n, s, err = testCP932.EncodeReplace(dst, []byte("A😀あ"), true)
	if err != nil || s != len("A😀あ") || !bytes.Equal(dst[:n], []byte{'A', 0xfc, 0xfc, 0x82, 0xa0}) {
		t.Fatalf("encode: %x %d %v", dst[:n], s, err)
	}
	n, s, err = testCP932.EncodeReplace(dst[:2], []byte("A😀"), true)
	if n != 1 || s != 1 || err != ErrShortDst {
		t.Fatalf("short encode replacement: %d %d %v", n, s, err)
	}
	n, s, err = testCP932.EncodeReplace(dst, []byte{0xe3, 0x81}, false)
	if n != 0 || s != 0 || err != ErrShortSrc {
		t.Fatalf("incomplete UTF-8: %d %d %v", n, s, err)
	}
	n, s, err = testCP932.EncodeReplace(dst, []byte{0xff, 0xe3, 0x81}, true)
	if err != nil || s != 3 || !bytes.Equal(dst[:n], []byte{0xfc, 0xfc, 0xfc, 0xfc, 0xfc, 0xfc}) {
		t.Fatalf("invalid UTF-8: %x %d %v", dst[:n], s, err)
	}
}
