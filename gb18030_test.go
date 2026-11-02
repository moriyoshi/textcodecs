package textcodecs

import (
	"bytes"
	"testing"
)

func TestGB18030AlgorithmicRanges(t *testing.T) {
	for _, r := range gb18030Ranges {
		for _, cp := range []rune{r.first, r.last} {
			key, ok := gbEncode(cp)
			if !ok {
				t.Fatalf("encode U+%04X", cp)
			}
			got, ok := gbDecode(key)
			if !ok || got != cp {
				t.Fatalf("range U+%04X -> %08x -> U+%04X", cp, key, got)
			}
		}
	}
	c, ok := Open("gb18030-2022")
	if !ok {
		t.Fatal("missing GB18030")
	}
	dst := make([]byte, 32)
	for _, tc := range []struct {
		text    string
		encoded []byte
	}{{"𐀀", []byte{0x90, 0x30, 0x81, 0x30}}, {"\U0010ffff", []byte{0xe3, 0x32, 0x9a, 0x35}}, {"ḿ", []byte{0xa8, 0xbc}}, {"\ue7c7", []byte{0x81, 0x35, 0xf4, 0x37}}, {"\ufffe", []byte{0x84, 0x31, 0xa4, 0x38}}, {"\uffff", []byte{0x84, 0x31, 0xa4, 0x39}}} {
		n, used, err := c.Encode(dst, []byte(tc.text), true)
		if err != nil || used != len(tc.text) || !bytes.Equal(dst[:n], tc.encoded) {
			t.Fatalf("encode %U: %x %d %v", []rune(tc.text)[0], dst[:n], used, err)
		}
		n, used, err = c.Decode(dst, tc.encoded, true)
		if err != nil || used != len(tc.encoded) || string(dst[:n]) != tc.text {
			t.Fatalf("decode %x: %q %d %v", tc.encoded, dst[:n], used, err)
		}
	}
	n, used, err := c.Encode(dst, []byte("\ue5e5"), true)
	if err != nil || used != len("\ue5e5") || !bytes.Equal(dst[:n], []byte{0xa3, 0xa0}) {
		t.Fatalf("GBK fallback: %x %d %v", dst[:n], used, err)
	}
	n, used, err = c.Decode(dst, []byte{0xa3, 0xa0}, true)
	if err != nil || used != 2 || string(dst[:n]) != "　" {
		t.Fatalf("GBK reverse fallback: %q %d %v", dst[:n], used, err)
	}
	for split := 0; split < 4; split++ {
		_, used, err = c.Decode(dst, []byte{0x90, 0x30, 0x81, 0x30}[:split], false)
		if split == 0 {
			if used != 0 || err != nil {
				t.Fatalf("empty input %d %v", used, err)
			}
		} else if used != 0 || err != ErrShortSrc {
			t.Fatalf("split %d: %d %v", split, used, err)
		}
	}
}
