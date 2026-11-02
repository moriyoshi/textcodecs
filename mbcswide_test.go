package textcodecs

import (
	"bytes"
	"strings"
	"testing"
)

func TestWideMultibyteRegistry(t *testing.T) {
	names := WideMultibyteNames()
	if len(names) != 34 {
		t.Fatalf("got %d converters", len(names))
	}
	for i, name := range names {
		if i > 0 && strings.ToLower(names[i-1]) >= strings.ToLower(name) {
			t.Fatalf("unsorted: %q %q", names[i-1], name)
		}
		c, ok := LookupWideMultibyte(strings.ToUpper(name))
		if !ok || c.entry.name != name {
			t.Fatalf("lookup %s", name)
		}
	}
	if _, ok := LookupWideMultibyte("not-an-encoding"); ok {
		t.Fatal("unknown encoding accepted")
	}
}

func TestWideBitmapRanks(t *testing.T) {
	for meta := wideSharedEncodeMeta; meta < wideSharedEncodeData; meta += 8 {
		off := wideSharedEncodeData + int(wide32(meta))
		count := 0
		for value := 0; value < 256; value++ {
			set := mbcsWideData[off+value/8]&(1<<uint(value%8)) != 0
			rank, ok := wideRank(off, byte(value))
			if ok != set || (set && rank != count) {
				t.Fatalf("group at %d, value %d: rank %d present %t, want rank %d present %t", off, value, rank, ok, count, set)
			}
			if set {
				count++
			}
		}
	}
}
func TestWideMultibyteStreaming(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		encoded    []byte
	}{
		{"euc-jp-2007", "Aあ丂", []byte{'A', 0xa4, 0xa2, 0x8f, 0xb0, 0xa1}},
		{"euc-jp-2007", "Aｱ", []byte{'A', 0x8e, 0xb1}},
		{"ibm-1375_P100-2008", "A\U00020021", []byte{'A', 0x9c, 0x71}},
		{"euc-tw-2014", "A丂𠀀", []byte{'A', 0x8e, 0xa4, 0xa1, 0xa6, 0x8e, 0xa5, 0xa1, 0xa5}},
	} {
		c, ok := LookupWideMultibyte(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		out := make([]byte, 64)
		n, used, err := c.Encode(out, []byte(tc.text), true)
		if err != nil || used != len(tc.text) || !bytes.Equal(out[:n], tc.encoded) {
			t.Fatalf("%s encode %x %d %v", tc.name, out[:n], used, err)
		}
		for split := 0; split <= len(tc.encoded); split++ {
			n1, used, err := c.Decode(out, tc.encoded[:split], false)
			if err != nil && err != ErrShortSrc {
				t.Fatalf("split %d: %v", split, err)
			}
			n2, used2, err := c.Decode(out[n1:], tc.encoded[used:], true)
			if err != nil || used+used2 != len(tc.encoded) || string(out[:n1+n2]) != tc.text {
				t.Fatalf("decode split %d: %q %d %d %v", split, out[:n1+n2], used, used2, err)
			}
		}
		for split := 0; split <= len(tc.text); split++ {
			src := []byte(tc.text)
			n1, used, err := c.Encode(out, src[:split], false)
			if err != nil && err != ErrShortSrc {
				t.Fatalf("split %d: %v", split, err)
			}
			n2, used2, err := c.Encode(out[n1:], src[used:], true)
			if err != nil || used+used2 != len(src) || !bytes.Equal(out[:n1+n2], tc.encoded) {
				t.Fatalf("encode split %d: %x %d %d %v", split, out[:n1+n2], used, used2, err)
			}
		}
		n, used, err = c.Decode(out, tc.encoded[1:2], true)
		if n != 0 || used != 0 || err != ErrInvalid {
			t.Fatalf("truncated EOF: %d %d %v", n, used, err)
		}
		n, used, err = c.Encode(out[:1], []byte(tc.text[1:]), true)
		if n != 0 || used != 0 || err != ErrShortDst {
			t.Fatalf("short destination: %d %d %v", n, used, err)
		}
	}
}

func TestEUCJPSingleShift2Incomplete(t *testing.T) {
	c, ok := Open("euc-jp-2007")
	if !ok {
		t.Fatal("missing EUC-JP converter")
	}
	out := make([]byte, 8)
	for _, tc := range []struct {
		atEOF bool
		want  error
	}{{false, ErrShortSrc}, {true, ErrInvalid}} {
		n, used, err := c.Decode(out, []byte{0x8e}, tc.atEOF)
		if n != 0 || used != 0 || err != tc.want {
			t.Fatalf("atEOF=%t: got %d bytes, consumed %d, error %v; want %v", tc.atEOF, n, used, err, tc.want)
		}
	}
}
func TestICUReservedDecodeSentinels(t *testing.T) {
	c, ok := LookupWideMultibyte("icu-internal-compound-t")
	if !ok {
		t.Fatal("missing converter")
	}
	out := make([]byte, 8)
	for _, s := range []string{"\ufffe", "\uffff"} {
		n, used, err := c.Encode(out, []byte(s), true)
		if err != nil || used != len(s) || n != 3 {
			t.Fatalf("encode %U: %x %d %v", []rune(s)[0], out[:n], used, err)
		}
		src := append([]byte(nil), out[:n]...)
		n, used, err = c.Decode(out, src, true)
		if n != 0 || used != 0 || err != ErrUnmapped {
			t.Fatalf("decode reserved: %d %d %v", n, used, err)
		}
	}
}

func BenchmarkWideMultibyte(b *testing.B) {
	for _, tc := range []struct{ name, text string }{{"euc-jp-2007", "日本語の文章を変換します。"}, {"gb18030-2022", "中国文字与扩展字符𠀀的转换。"}} {
		c, ok := LookupWideMultibyte(tc.name)
		if !ok {
			b.Fatal(tc.name)
		}
		src := bytes.Repeat([]byte(tc.text), 256)
		dst := make([]byte, len(src)*2)
		n, _, err := c.Encode(dst, src, true)
		if err != nil {
			b.Fatal(err)
		}
		encoded := append([]byte(nil), dst[:n]...)
		b.Run(tc.name+"/encode", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := c.Encode(dst, src, true)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(tc.name+"/decode", func(b *testing.B) {
			b.SetBytes(int64(len(encoded)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _, err := c.Decode(dst, encoded, true)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
