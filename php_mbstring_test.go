package textcodecs

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestPHPMappings(t *testing.T) {
	lineRE := regexp.MustCompile(`^((?:<U[0-9A-F]+>)+) ((?:\\x[0-9A-F]{2})+) \|([034])`)
	runeRE := regexp.MustCompile(`<U([0-9A-F]+)>`)
	for _, list := range []string{"registry-sbcs.txt", "registry-wide.txt"} {
		raw, err := os.ReadFile(filepath.Join("data/php-mbstring", list))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range strings.Fields(string(raw)) {
			t.Run(file, func(t *testing.T) {
				c, ok := Open(strings.TrimSuffix(file, ".ucm"))
				if !ok {
					t.Fatal("codec is missing")
				}
				f, err := os.Open(filepath.Join("data/php-mbstring", file))
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				s := bufio.NewScanner(f)
				count := 0
				for s.Scan() {
					m := lineRE.FindStringSubmatch(s.Text())
					if m == nil {
						continue
					}
					encoded, err := hex.DecodeString(strings.ReplaceAll(m[2], `\x`, ""))
					if err != nil {
						t.Fatal(err)
					}
					var rs []rune
					for _, part := range runeRE.FindAllStringSubmatch(m[1], -1) {
						n, err := strconv.ParseUint(part[1], 16, 32)
						if err != nil {
							t.Fatal(err)
						}
						rs = append(rs, rune(n))
					}
					text := []byte(string(rs))
					var dst [64]byte
					if m[3] != "4" {
						c.Reset()
						n, used, err := c.Decode(dst[:], encoded, true)
						if err != nil || used != len(encoded) || !bytes.Equal(dst[:n], text) {
							t.Fatalf("decode %x: %x, %d, %v; want %x", encoded, dst[:n], used, err, text)
						}
					}
					if m[3] != "3" {
						c.Reset()
						n, used, err := c.Encode(dst[:], text, true)
						if err != nil || used != len(text) || !bytes.Equal(dst[:n], encoded) {
							t.Fatalf("encode %U: %x, %d, %v; want %x", rs, dst[:n], used, err, encoded)
						}
					}
					count++
				}
				if err := s.Err(); err != nil {
					t.Fatal(err)
				}
				if count == 0 {
					t.Fatal("no mappings")
				}
			})
		}
	}
}

func TestPHPMultiscalarStreaming(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		encoded    []byte
	}{
		{"php-sjis-mac", "\uf862有限会社", []byte{0x87, 0xfb}},
		{"php-sjis-mobile-docomo", "#\u20e3", []byte{0xf9, 0x85}},
	} {
		c, ok := Open(tc.name)
		if !ok {
			t.Fatal(tc.name)
		}
		var out [32]byte
		first := []byte(string([]rune(tc.text)[0]))
		n, used, err := c.Encode(out[:], first, false)
		if n != 0 || used != 0 || err != ErrShortSrc {
			t.Fatalf("%s: prefix %d %d %v", tc.name, n, used, err)
		}
		n, used, err = c.Encode(out[:], []byte(tc.text), true)
		if err != nil || used != len(tc.text) || !bytes.Equal(out[:n], tc.encoded) {
			t.Fatalf("%s: %x %d %v", tc.name, out[:n], used, err)
		}
		n, used, err = c.Decode(out[:len([]byte(tc.text))-1], tc.encoded, true)
		if n != 0 || used != 0 || err != ErrShortDst {
			t.Fatalf("%s: short destination %d %d %v", tc.name, n, used, err)
		}
	}
}

func TestPHPJIS2004DiffersFromProjectX0213(t *testing.T) {
	for _, tc := range []struct {
		php, project string
		encoded      []byte
		phpText      string
		projectText  string
	}{
		{"php-sjis-2004", "shift_jis-2004", []byte{0x81, 0x5f}, "\uff3c", "\\"},
		{"php-sjis-2004", "shift_jis-2004", []byte{0x81, 0xb0}, "\uff5e", "~"},
		{"php-euc-jp-2004", "euc-jis-2004", []byte{0xa1, 0xb1}, "\uffe3", "\u203e"},
		{"php-euc-jp-2004", "euc-jis-2004", []byte{0xa1, 0xef}, "\uffe5", "\u00a5"},
	} {
		for _, source := range []struct{ name, want string }{{tc.php, tc.phpText}, {tc.project, tc.projectText}} {
			c, ok := Open(source.name)
			if !ok {
				t.Fatal(source.name)
			}
			var out [16]byte
			n, used, err := c.Decode(out[:], tc.encoded, true)
			if err != nil || used != len(tc.encoded) || string(out[:n]) != source.want {
				t.Fatalf("%s decode %x: %q, %d, %v; want %q", source.name, tc.encoded, out[:n], used, err, source.want)
			}
		}
	}
	for _, tc := range []struct {
		php, project, text     string
		phpBytes, projectBytes []byte
	}{
		{"php-sjis-2004", "shift_jis-2004", "\\", []byte{0x5c}, []byte{0x81, 0x5f}},
		{"php-sjis-2004", "shift_jis-2004", "~", []byte{0x7e}, []byte{0x81, 0xb0}},
		{"php-euc-jp-2004", "euc-jis-2004", "\u00a5", []byte{0x5c}, []byte{0xa1, 0xef}},
		{"php-euc-jp-2004", "euc-jis-2004", "\u203e", []byte{0x7e}, []byte{0xa1, 0xb1}},
	} {
		for _, source := range []struct {
			name string
			want []byte
		}{{tc.php, tc.phpBytes}, {tc.project, tc.projectBytes}} {
			c, ok := Open(source.name)
			if !ok {
				t.Fatal(source.name)
			}
			var out [16]byte
			n, used, err := c.Encode(out[:], []byte(tc.text), true)
			if err != nil || used != len(tc.text) || !bytes.Equal(out[:n], source.want) {
				t.Fatalf("%s encode %U: %x, %d, %v; want %x", source.name, []rune(tc.text), out[:n], used, err, source.want)
			}
		}
	}
}

func TestPHPOtherMappingsDifferFromICU(t *testing.T) {
	for _, tc := range []struct {
		php, icu         string
		bytes            []byte
		phpText, icuText string
	}{
		{"php-cp850", "ibm-850_P100-1995", []byte{0x1a}, "\u001a", "\u001c"},
		{"php-cp866", "ibm-866_P100-1995", []byte{0x1a}, "\u001a", "\u001c"},
		{"php-big5", "windows-950-2000", []byte{0x80}, "", "\u0080"},
		{"php-cp950", "windows-950-2000", []byte{0xa3, 0xe1}, "", "\u20ac"},
		{"php-euc-cn", "ibm-1383_P110-1999", []byte{0xa3, 0xa7}, "\uff07", "\u00b4"},
		{"php-euc-kr", "ibm-970_P110_P110-2006_U2", []byte{0xa2, 0xe6}, "\u20ac", ""},
		{"php-euc-tw", "euc-tw-2014", []byte{0x8e, 0xa1, 0xa1, 0xf0}, "\u00a7", ""},
	} {
		for _, side := range []struct{ name, want string }{{tc.php, tc.phpText}, {tc.icu, tc.icuText}} {
			c, ok := Open(side.name)
			if !ok {
				t.Fatal(side.name)
			}
			var dst [16]byte
			n, used, err := c.Decode(dst[:], tc.bytes, true)
			if side.want == "" {
				if err == nil {
					t.Fatalf("%s unexpectedly decoded %x as %q", side.name, tc.bytes, dst[:n])
				}
				continue
			}
			if err != nil || used != len(tc.bytes) || string(dst[:n]) != side.want {
				t.Fatalf("%s decode %x: %q %d %v", side.name, tc.bytes, dst[:n], used, err)
			}
		}
	}
	for _, tc := range []struct {
		php, icu, input    string
		phpBytes, icuBytes []byte
	}{
		{"php-cp936", "windows-936-2000", "\u203e", []byte{0xa3, 0xfe}, nil},
		{"php-cp950", "windows-950-2000", "\u2550", []byte{0xa2, 0xa4}, []byte{0xf9, 0xf9}},
	} {
		for _, side := range []struct {
			name string
			want []byte
		}{{tc.php, tc.phpBytes}, {tc.icu, tc.icuBytes}} {
			c, ok := Open(side.name)
			if !ok {
				t.Fatal(side.name)
			}
			var dst [16]byte
			n, used, err := c.Encode(dst[:], []byte(tc.input), true)
			if side.want == nil {
				if err == nil && n != 0 {
					t.Fatalf("%s unexpectedly encoded %U as %x", side.name, []rune(tc.input), dst[:n])
				}
				continue
			}
			if err != nil || used != len(tc.input) || !bytes.Equal(dst[:n], side.want) {
				t.Fatalf("%s encode %U: %x %d %v", side.name, []rune(tc.input), dst[:n], used, err)
			}
		}
	}
}

func TestPHPRemainingSingleByteMappingsDifferFromICU(t *testing.T) {
	for _, tc := range []struct {
		php, icu         string
		b                byte
		phpText, icuText string
	}{
		{"php-ascii", "ibm-1089_P100-1995", 0x80, "", "\u0080"},
		{"php-armscii-8", "ibm-1089_P100-1995", 0xa2, "\u0587", ""},
		{"php-iso-8859-1", "ibm-922_P100-1999", 0xd0, "\u00d0", "\u0160"},
		{"php-iso-8859-16", "ibm-923_P100-1998", 0xa1, "\u0104", "\u00a1"},
		{"php-windows-1251", "ibm-5347_P100-1998", 0x98, "", "\u0098"},
		{"php-windows-1254", "ibm-5350_P100-1998", 0x81, "", "\u0081"},
	} {
		for _, side := range []struct{ name, want string }{{tc.php, tc.phpText}, {tc.icu, tc.icuText}} {
			c, ok := Open(side.name)
			if !ok {
				t.Fatal(side.name)
			}
			var dst [8]byte
			n, used, err := c.Decode(dst[:], []byte{tc.b}, true)
			if side.want == "" {
				if err == nil {
					t.Fatalf("%s unexpectedly decoded %02x as %q", side.name, tc.b, dst[:n])
				}
				continue
			}
			if err != nil || used != 1 || string(dst[:n]) != side.want {
				t.Fatalf("%s decode %02x: %q %d %v", side.name, tc.b, dst[:n], used, err)
			}
		}
	}
}

func TestPHPDuplicateSingleByteMappings(t *testing.T) {
	duplicates := map[string]string{
		"iso-8859-2": "ibm-912_P100-1995", "iso-8859-3": "ibm-913_P100-2000",
		"iso-8859-4": "ibm-914_P100-1995", "iso-8859-5": "ibm-915_P100-1995",
		"iso-8859-6": "ibm-1089_P100-1995", "iso-8859-7": "ibm-9005_X110-2007",
		"iso-8859-8": "ibm-5012_P100-1999", "iso-8859-9": "ibm-920_P100-1995",
		"iso-8859-10": "iso-8859_10-1998", "iso-8859-13": "ibm-921_P100-1995",
		"iso-8859-14": "iso-8859_14-1998", "iso-8859-15": "ibm-923_P100-1998",
		"koi8-r": "ibm-878_P100-1996", "koi8-u": "ibm-1168_P100-2002", "windows-1252": "ibm-5348_P100-1997",
	}
	re := regexp.MustCompile(`^<U([0-9A-F]+)> \\x([0-9A-F]{2}) \|([034])`)
	for php, existing := range duplicates {
		t.Run(php, func(t *testing.T) {
			f, err := os.Open(filepath.Join("data/php-mbstring", "php-"+php+".ucm"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			decode := map[byte]rune{}
			encode := map[rune]byte{}
			s := bufio.NewScanner(f)
			for s.Scan() {
				m := re.FindStringSubmatch(s.Text())
				if m == nil {
					continue
				}
				r, _ := strconv.ParseUint(m[1], 16, 32)
				b, _ := strconv.ParseUint(m[2], 16, 8)
				if m[3] != "4" {
					decode[byte(b)] = rune(r)
				}
				if m[3] != "3" {
					encode[rune(r)] = byte(b)
				}
			}
			if err := s.Err(); err != nil {
				t.Fatal(err)
			}
			c, ok := Open(existing)
			if !ok {
				t.Fatal(existing)
			}
			var dst [8]byte
			for b := 0; b < 256; b++ {
				c.Reset()
				n, used, err := c.Decode(dst[:], []byte{byte(b)}, true)
				want, present := decode[byte(b)]
				if present {
					if err != nil || used != 1 || string(dst[:n]) != string(want) {
						t.Fatalf("decode %02x: %x %v", b, dst[:n], err)
					}
				} else if err == nil {
					t.Fatalf("unexpected byte %02x", b)
				}
			}
			for r := rune(0); r <= 0xffff; r++ {
				if r >= 0xd800 && r <= 0xdfff {
					continue
				}
				c.Reset()
				n, _, err := c.Encode(dst[:], []byte(string(r)), true)
				want, present := encode[r]
				if present {
					if err != nil || n != 1 || dst[0] != want {
						t.Fatalf("encode %U: %x %v", r, dst[:n], err)
					}
				} else if err == nil && n > 0 {
					t.Fatalf("unexpected encoding for %U", r)
				}
			}
		})
	}
}
