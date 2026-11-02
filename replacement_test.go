package textcodecs

import (
	"bytes"
	"testing"
)

func TestMultibyteReplacementBytes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		replacement []byte
	}{
		{"ibm-943_P15A-2003", []byte{0xfc, 0xfc}},
		{"windows-936-2000", []byte{0x3f}},
		{"ibm-1386_P100-2001", []byte{0xa1, 0xa1}},
	} {
		c, ok := LookupMultibyte(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		dst := make([]byte, 8)
		n, used, err := c.EncodeReplace(dst, []byte("😀"), true)
		if err != nil || used != len("😀") || !bytes.Equal(dst[:n], tc.replacement) {
			t.Fatalf("%s: %x %d %v", tc.name, dst[:n], used, err)
		}
		n, used, err = c.EncodeReplace(dst[:len(tc.replacement)-1], []byte("😀"), true)
		if n != 0 || used != 0 || err != ErrShortDst {
			t.Fatalf("%s short output: %d %d %v", tc.name, n, used, err)
		}
	}
}
