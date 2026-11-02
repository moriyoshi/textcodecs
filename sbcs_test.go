package textcodecs

import (
	"bytes"
	"strings"
	"testing"
)

func TestSingleByteRegistry(t *testing.T) {
	names := SingleByteNames()
	if len(names) != 155 {
		t.Fatalf("got %d codecs", len(names))
	}
	for i, name := range names {
		if i > 0 && strings.Compare(strings.ToLower(names[i-1]), strings.ToLower(name)) >= 0 {
			t.Fatalf("registry not sorted: %q, %q", names[i-1], name)
		}
		c, ok := LookupSingleByte(strings.ToUpper(name))
		if !ok || c.entry.name != name {
			t.Fatalf("case-insensitive lookup: %s", name)
		}
	}
	if _, ok := LookupSingleByte("not-an-encoding"); ok {
		t.Fatal("unknown codec accepted")
	}
	names[0] = "mutated"
	if SingleByteNames()[0] == "mutated" {
		t.Fatal("names expose internal state")
	}
}

func TestSingleByteExamplesAndStreaming(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		encoded    []byte
	}{
		{"ibm-5348_P100-1997", "A€Z", []byte{'A', 0x80, 'Z'}},
		{"ibm-37_P100-1995", "Hello", []byte{0xc8, 0x85, 0x93, 0x93, 0x96}},
	} {
		c, ok := LookupSingleByte(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		dst := make([]byte, 32)
		n, used, err := c.Encode(dst, []byte(tc.text), true)
		if err != nil || used != len(tc.text) || !bytes.Equal(dst[:n], tc.encoded) {
			t.Fatalf("%s encode %x %d %v", tc.name, dst[:n], used, err)
		}
		n, used, err = c.Decode(dst, tc.encoded)
		if err != nil || used != len(tc.encoded) || string(dst[:n]) != tc.text {
			t.Fatalf("%s decode %q %d %v", tc.name, dst[:n], used, err)
		}
		n, used, err = c.Encode(dst[:1], []byte(tc.text), true)
		if err != ErrShortDst || n != 1 || used != 1 {
			t.Fatalf("%s short dst: %d %d %v", tc.name, n, used, err)
		}
	}
	c, _ := LookupSingleByte("ibm-5348_P100-1997")
	dst := make([]byte, 8)
	n, used, err := c.Encode(dst, []byte{'A', 0xe2, 0x82}, false)
	if n != 1 || used != 1 || err != ErrShortSrc {
		t.Fatalf("short source: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(dst[:2], []byte{0x80})
	if n != 0 || used != 0 || err != ErrShortDst {
		t.Fatalf("decode short: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(dst, []byte("☃"), true)
	if n != 0 || used != 0 || err != ErrUnmapped {
		t.Fatalf("unmapped: %d %d %v", n, used, err)
	}
}
