package textcodecs

import (
	"bytes"
	"errors"
	"testing"
)

func TestErrorModesSingleByte(t *testing.T) {
	// ebcdik has no mapping for 0xb2; 0x40 maps to a space.
	src := []byte{0x40, 0xb2, 0x40}
	for _, tc := range []struct {
		mode ErrorMode
		want string
		used int
		err  error
	}{
		{ErrorStrict, " ", 1, ErrUnmapped},
		{ErrorReplace, " � ", 3, nil},
		{ErrorIgnore, "  ", 3, nil},
	} {
		c, err := OpenWithOptions("ebcdik", ErrorOptions{DecodeMode: tc.mode})
		if err != nil {
			t.Fatal(err)
		}
		var dst [32]byte
		n, used, err := c.Decode(dst[:], src, true)
		if string(dst[:n]) != tc.want || used != tc.used || err != tc.err {
			t.Fatalf("mode %d: %q %d %v", tc.mode, dst[:n], used, err)
		}
	}
	c, err := OpenWithOptions("ebcdik", ErrorOptions{EncodeMode: ErrorReplace, DecodeMode: ErrorReplace, EncodeReplacement: "!", DecodeReplacement: "※"})
	if err != nil {
		t.Fatal(err)
	}
	var dst [32]byte
	n, used, err := c.Decode(dst[:], src, true)
	if string(dst[:n]) != " ※ " || used != len(src) || err != nil {
		t.Fatalf("custom decode: %q %d %v", dst[:n], used, err)
	}
	n, used, err = c.Decode(dst[:2], []byte{0xb2}, true)
	if n != 0 || used != 0 || err != ErrShortDst {
		t.Fatalf("short replacement: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(dst[:], []byte("😀"), true)
	if err != nil || used != len("😀") || n != 1 || dst[0] != 0x4f {
		t.Fatalf("custom encode: %x %d %v", dst[:n], used, err)
	}
	n, used, err = c.Encode(dst[:0], []byte("😀"), true)
	if n != 0 || used != 0 || err != ErrShortDst {
		t.Fatalf("short encode: %d %d %v", n, used, err)
	}
}

func TestErrorOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options ErrorOptions
		want    error
	}{
		{"missing", ErrorOptions{}, ErrUnknownEncoding},
		{"ebcdik", ErrorOptions{DecodeMode: ErrorMode(8)}, ErrInvalidOptions},
		{"ebcdik", ErrorOptions{EncodeMode: ErrorReplace, EncodeReplacement: "ab"}, ErrInvalidOptions},
		{"ebcdik", ErrorOptions{DecodeMode: ErrorReplace, DecodeReplacement: "\xff"}, ErrInvalidOptions},
		{"ebcdik", ErrorOptions{EncodeMode: ErrorReplace, EncodeReplacement: "😀"}, ErrInvalidOptions},
	} {
		_, err := OpenWithOptions(tc.name, tc.options)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s %#v: %v, want %v", tc.name, tc.options, err, tc.want)
		}
	}
}

func TestErrorModesMultibyte(t *testing.T) {
	for _, mode := range []ErrorMode{ErrorReplace, ErrorIgnore} {
		c, err := OpenWithOptions("ibm-943_P15A-2003", ErrorOptions{EncodeMode: mode, DecodeMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		in := []byte{'A', 0x81, 0xad, 'B', 0xff}
		var out [32]byte
		n, used, err := c.Decode(out[:], in, true)
		want := "AB"
		if mode == ErrorReplace {
			want = "A�B�"
		}
		if err != nil || used != len(in) || string(out[:n]) != want {
			t.Fatalf("mode %d decode: %q %d %v", mode, out[:n], used, err)
		}
		in = []byte("A😀B")
		n, used, err = c.Encode(out[:], in, true)
		wantBytes := []byte("AB")
		if mode == ErrorReplace {
			wantBytes = []byte("A?B")
		}
		if err != nil || used != len(in) || !bytes.Equal(out[:n], wantBytes) {
			t.Fatalf("mode %d encode: %x %d %v", mode, out[:n], used, err)
		}
		n, used, err = c.Decode(out[:], []byte{0x81}, false)
		if n != 0 || used != 0 || err != ErrShortSrc {
			t.Fatalf("mode %d incomplete: %d %d %v", mode, n, used, err)
		}
	}
}

func TestErrorModesStatefulReplacement(t *testing.T) {
	name := "ibm-930_P120-1999"
	c, err := OpenWithOptions(name, ErrorOptions{EncodeMode: ErrorReplace})
	if err != nil {
		t.Fatal(err)
	}
	base, ok := Open(name)
	if !ok {
		t.Fatal(name)
	}
	src := []byte("あ😀い")
	var want, out [64]byte
	nw, _, err := base.Encode(want[:], []byte("あ?い"), true)
	if err != nil {
		t.Fatal(err)
	}
	n1, used1, err := c.Encode(out[:3], src, true)
	if err != ErrShortDst || n1 != 3 || used1 != len("あ") {
		t.Fatalf("short: %x %d %v", out[:n1], used1, err)
	}
	n2, used2, err := c.Encode(out[n1:], src[used1:], true)
	if err != nil || used1+used2 != len(src) || !bytes.Equal(out[:n1+n2], want[:nw]) {
		t.Fatalf("retry: %x %d+%d %v, want %x", out[:n1+n2], used1, used2, err, want[:nw])
	}
	c.Reset()
	n, used, err := c.Encode(out[:], src, true)
	if err != nil || used != len(src) || !bytes.Equal(out[:n], want[:nw]) {
		t.Fatalf("reset: %x %d %v", out[:n], used, err)
	}
}

func TestErrorModesWideUnmappedSequence(t *testing.T) {
	base, ok := Open("euc-jp-2007")
	if !ok {
		t.Fatal("missing EUC-JP")
	}
	var bad []byte
	var out [64]byte
	for a := 0xa1; a <= 0xfe && bad == nil; a++ {
		for b := 0xa1; b <= 0xfe; b++ {
			candidate := []byte{0x8f, byte(a), byte(b)}
			n, used, err := base.Decode(out[:], candidate, true)
			if n == 0 && used == 0 && err == ErrUnmapped {
				bad = candidate
				break
			}
		}
	}
	if bad == nil {
		t.Fatal("no unmapped three-byte sequence")
	}
	for _, mode := range []ErrorMode{ErrorReplace, ErrorIgnore} {
		c, err := OpenWithOptions("euc-jp-2007", ErrorOptions{DecodeMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		src := append(append([]byte(nil), bad...), 'A')
		n, used, err := c.Decode(out[:], src, true)
		want := "A"
		if mode == ErrorReplace {
			want = "�A"
		}
		if err != nil || used != len(src) || string(out[:n]) != want {
			t.Fatalf("mode %d decode %x: %q %d %v", mode, src, out[:n], used, err)
		}
	}
}

func TestErrorModesStatefulDecode(t *testing.T) {
	for _, name := range []string{"ibm-930_P120-1999", "ibm-1390_P110-2003"} {
		t.Run(name, func(t *testing.T) {
			base, ok := Open(name)
			if !ok {
				t.Fatal(name)
			}
			var bad []byte
			var out [32]byte
			for a := 0; a < 256 && bad == nil; a++ {
				for b := 0; b < 256; b++ {
					base.Reset()
					candidate := []byte{0x0e, byte(a), byte(b), 0x0f}
					n, used, err := base.Decode(out[:], candidate, true)
					if n == 0 && used == 1 && err == ErrUnmapped {
						bad = candidate
						break
					}
				}
			}
			if bad == nil {
				t.Fatal("no unmapped shifted pair")
			}
			for _, mode := range []ErrorMode{ErrorReplace, ErrorIgnore} {
				c, err := OpenWithOptions(name, ErrorOptions{DecodeMode: mode})
				if err != nil {
					t.Fatal(err)
				}
				n, used, err := c.Decode(out[:], bad, true)
				want := ""
				if mode == ErrorReplace {
					want = "�"
				}
				if err != nil || used != len(bad) || string(out[:n]) != want {
					t.Fatalf("mode %d decode %x: %q %d %v", mode, bad, out[:n], used, err)
				}
				// EOF must reset the decoder's shift state after the ignored/replaced pair.
				n, used, err = c.Decode(out[:], []byte{0xc1}, true)
				if err != nil || used != 1 || string(out[:n]) != "A" {
					t.Fatalf("after EOF: %q %d %v", out[:n], used, err)
				}
			}
		})
	}
}

func TestErrorModesPairPrefixReplacement(t *testing.T) {
	name := "ibm-1390_P110-2003"
	c, err := OpenWithOptions(name, ErrorOptions{EncodeMode: ErrorReplace, EncodeReplacement: "æ"})
	if err != nil {
		t.Fatal(err)
	}
	base, _ := Open(name)
	var want, got [64]byte
	nw, _, err := base.Encode(want[:], []byte("AæA"), true)
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("A😀A")
	n1, used1, err := c.Encode(got[:4], src, true)
	if err != ErrShortDst || n1 != 1 || used1 != 1 {
		t.Fatalf("short pair-prefix replacement: %x %d %v", got[:n1], used1, err)
	}
	n2, used2, err := c.Encode(got[n1:], src[used1:], true)
	if err != nil || used1+used2 != len(src) || !bytes.Equal(got[:n1+n2], want[:nw]) {
		t.Fatalf("retry: %x %d+%d %v; want %x", got[:n1+n2], used1, used2, err, want[:nw])
	}
}

func TestErrorModesGSMAndX0213(t *testing.T) {
	c, err := OpenWithOptions("gsm-03.38-2009", ErrorOptions{DecodeMode: ErrorReplace, DecodeReplacement: "💥"})
	if err != nil {
		t.Fatal(err)
	}
	var out [32]byte
	n, used, err := c.Decode(out[:], []byte{0x80, 0x41}, true)
	if err != nil || used != 2 || string(out[:n]) != "💥A" {
		t.Fatalf("GSM decode: %q %d %v", out[:n], used, err)
	}
	c, err = OpenWithOptions("euc-jis-2004", ErrorOptions{EncodeMode: ErrorReplace, EncodeReplacement: "か"})
	if err != nil {
		t.Fatal(err)
	}
	n, used, err = c.Encode(out[:], []byte("😀"), true)
	if err != nil || used != len("😀") || !bytes.Equal(out[:n], []byte{0xa4, 0xab}) {
		t.Fatalf("pair-prefix replacement: %x %d %v", out[:n], used, err)
	}
}

func TestErrorModesMalformedUTF8(t *testing.T) {
	for _, mode := range []ErrorMode{ErrorReplace, ErrorIgnore} {
		c, err := OpenWithOptions("ibm-943_P15A-2003", ErrorOptions{EncodeMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		var out [8]byte
		n, used, err := c.Encode(out[:], []byte{0xe3, 0x81}, false)
		if n != 0 || used != 0 || err != ErrShortSrc {
			t.Fatalf("mode %d partial: %d %d %v", mode, n, used, err)
		}
		n, used, err = c.Encode(out[:], []byte{0xe3, 0x81}, true)
		want := []byte(nil)
		if mode == ErrorReplace {
			want = []byte("??")
		}
		if err != nil || used != 2 || !bytes.Equal(out[:n], want) {
			t.Fatalf("mode %d EOF: %x %d %v", mode, out[:n], used, err)
		}
	}
}
