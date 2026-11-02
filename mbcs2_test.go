package textcodecs

import (
	"bytes"
	"strings"
	"testing"
)

func TestMultibyteRegistry(t *testing.T) {
	names := MultibyteNames()
	if len(names) != 28 {
		t.Fatalf("got %d converters", len(names))
	}
	for i, name := range names {
		if i > 0 && strings.ToLower(names[i-1]) >= strings.ToLower(name) {
			t.Fatalf("unsorted: %q %q", names[i-1], name)
		}
		c, ok := LookupMultibyte(strings.ToUpper(name))
		if !ok || c.entry.name != name {
			t.Fatalf("lookup %s", name)
		}
	}
	if _, ok := LookupMultibyte("not-an-encoding"); ok {
		t.Fatal("unknown encoding accepted")
	}
}

func TestMultibyteStreaming(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		encoded    []byte
	}{
		{"ibm-1386_P100-2001", "A中国", []byte{'A', 0xd6, 0xd0, 0xb9, 0xfa}},
		{"ibm-1363_P11B-1998", "A가나", []byte{'A', 0xb0, 0xa1, 0xb3, 0xaa}},
	} {
		c, ok := LookupMultibyte(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		out := make([]byte, 32)
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
		n, used, err = c.Decode(out[:2], tc.encoded[1:3], true)
		if n != 0 || used != 0 || err != ErrShortDst {
			t.Fatalf("short decode: %d %d %v", n, used, err)
		}
		n, used, err = c.Encode(out[:1], []byte(tc.text[1:]), true)
		if n != 0 || used != 0 || err != ErrShortDst {
			t.Fatalf("short encode: %d %d %v", n, used, err)
		}
		n, used, err = c.Decode(out, tc.encoded[1:2], true)
		if n != 0 || used != 0 || err != ErrInvalid {
			t.Fatalf("truncated EOF: %d %d %v", n, used, err)
		}
	}
}

func BenchmarkMultibyteChinese(b *testing.B) {
	c, ok := LookupMultibyte("windows-936-2000")
	if !ok {
		b.Fatal("missing converter")
	}
	src := bytes.Repeat([]byte("中国文字编码转换测试与ASCII混合。"), 256)
	dst := make([]byte, len(src)*2)
	n, _, err := c.Encode(dst, src, true)
	if err != nil {
		b.Fatal(err)
	}
	encoded := append([]byte(nil), dst[:n]...)
	b.Run("encode", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _, err := c.Encode(dst, src, true)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("decode", func(b *testing.B) {
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
