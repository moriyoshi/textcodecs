package textcodecs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEBCDICTables(t *testing.T) {
	paths, err := filepath.Glob("data/ebcdic-kana/*.ucm")
	if err != nil || len(paths) != 9 {
		t.Fatalf("source files: %v, %v", paths, err)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".ucm")
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var decoded [256]rune
			var present [256]bool
			encoded := map[rune]byte{}
			foundName := false
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "<code_set_name>") {
					fields := strings.Fields(line)
					if len(fields) != 2 {
						t.Fatalf("bad code-set name %q", line)
					}
					declared, err := strconv.Unquote(fields[1])
					if err != nil || declared != name {
						t.Fatalf("code-set name %q: %v; want %q", fields[1], err, name)
					}
					foundName = true
				}
				if !strings.HasPrefix(line, "<U") {
					continue
				}
				var r, b, precision uint32
				if n, err := fmt.Sscanf(line, `<U%x> \x%x |%d`, &r, &b, &precision); err != nil || n != 3 || b > 255 || precision != 0 {
					t.Fatalf("bad mapping %q: %v", line, err)
				}
				if present[b] {
					t.Fatalf("duplicate byte %x", b)
				}
				if _, exists := encoded[rune(r)]; exists {
					t.Fatalf("duplicate scalar U+%04X", r)
				}
				decoded[b], present[b] = rune(r), true
				encoded[rune(r)] = byte(b)
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if !foundName {
				t.Fatal("missing code-set name")
			}
			codec, ok := LookupSingleByte(name)
			if !ok {
				t.Fatal("missing codec")
			}
			var out [8]byte
			for b := 0; b < 256; b++ {
				n, used, err := codec.Decode(out[:], []byte{byte(b)})
				if !present[b] {
					if n != 0 || used != 0 || err != ErrUnmapped {
						t.Fatalf("decode %02x: %x %d %v", b, out[:n], used, err)
					}
				} else if err != nil || used != 1 || string(out[:n]) != string(decoded[b]) {
					t.Fatalf("decode %02x: %x %d %v; want U+%04X", b, out[:n], used, err, decoded[b])
				}
			}
			for r, want := range encoded {
				src := []byte(string(r))
				n, used, err := codec.Encode(out[:], src, true)
				if err != nil || used != len(src) || n != 1 || out[0] != want {
					t.Fatalf("encode U+%04X: %x %d %v; want %02x", r, out[:n], used, err, want)
				}
			}
			if _, ok := Open(strings.ToUpper(name)); !ok {
				t.Fatal("registry does not expose this mapping")
			}
		})
	}
}

func TestEBCDICKanaNames(t *testing.T) {
	for _, name := range []string{
		"ebcdic-nec", "ebcdic-nec_lf-nl-alt", "ebcdik", "ebcdik_lf-nl-alt",
		"ibm-290_hitachi-flavored", "ibm-290_tilde",
		"ibm-298", "ibm-298_backslash", "ibm-298_hitachi-flavored",
	} {
		if _, ok := Open(name); !ok {
			t.Errorf("missing renamed codec %s", name)
		}
	}
	for _, name := range []string{
		"ebcdic-nec-lf-nl-alt", "ebcdik-lf-nl-alt", "ibm290", "ibm-290",
		"ibm290-hitachi-flavored", "ibm290-tilde", "ibm298",
		"ibm298-backslash", "ibm298-hitachi-flavored",
	} {
		if _, ok := Open(name); ok {
			t.Errorf("old name %s remains registered", name)
		}
	}
}

func TestEBCDICKanaVariants(t *testing.T) {
	cases := []struct {
		name string
		b    byte
		r    rune
	}{
		{"ebcdik", 0x15, '\u0085'}, {"ebcdik_lf-nl-alt", 0x15, '\n'},
		{"ebcdic-nec", 0x15, '\u0085'}, {"ebcdic-nec_lf-nl-alt", 0x15, '\n'},
		{"ibm-8482_P100-1999", 0xe1, '€'}, {"ibm-298_backslash", 0xe0, '\\'},
	}
	for _, tc := range cases {
		c, ok := Open(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		var out [8]byte
		n, used, err := c.Decode(out[:], []byte{tc.b}, true)
		if err != nil || used != 1 || string(out[:n]) != string(tc.r) {
			t.Errorf("%s: %q %d %v", tc.name, out[:n], used, err)
		}
	}
}

func TestIBM290Variants(t *testing.T) {
	var a [8]byte
	for _, tc := range []struct {
		name string
		b    byte
		want string
		err  error
	}{
		{"ibm-290_P100-1995", 0xe1, "", ErrUnmapped},
		{"ibm-8482_P100-1999", 0xe1, "€", nil},
		{"ibm-290_tilde", 0xe1, "", ErrUnmapped},
		{"ibm-290_P100-1995", 0xa0, "~", nil},
		{"ibm-290_P100-1995", 0xa1, "‾", nil},
		{"ibm-290_tilde", 0xa0, "", ErrUnmapped},
		{"ibm-290_tilde", 0xa1, "~", nil},
	} {
		c, ok := LookupSingleByte(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		n, used, err := c.Decode(a[:], []byte{tc.b})
		if err != tc.err || string(a[:n]) != tc.want || (err == nil && used != 1) || (err != nil && used != 0) {
			t.Errorf("%s %02x: %q %d %v", tc.name, tc.b, a[:n], used, err)
		}
	}
}
