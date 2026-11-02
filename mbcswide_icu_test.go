//go:build icu

package textcodecs

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWideMultibyteICUDifferential(t *testing.T) {
	dir := t.TempDir()
	inputs, err := filepath.Glob("data/icu/mbcswide/*.ucm")
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"-d", dir}, inputs...)
	if out, err := exec.Command("makeconv", args...).CombinedOutput(); err != nil {
		t.Fatalf("makeconv: %v: %s", err, out)
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "icu-uc").Output()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "wide-oracle")
	args = []string{"-O2", "-o", exe, "testdata/mbcswide_oracle.c"}
	args = append(args, bytesToFields(flags)...)
	if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	for _, name := range WideMultibyteNames() {
		if name == "euc-jis-2004" || name == "shift_jis-2004" || name == "nta-jis-x-0213-euc" || name == "nta-jis-x-0213-sjis" || strings.HasPrefix(name, "php-") {
			continue // These mappings come from external sources, not the ICU UCM directory.
		}
		t.Run(name, func(t *testing.T) {
			codec, ok := LookupWideMultibyte(name)
			if !ok {
				t.Fatal(name)
			}
			e := codec.entry
			var queries bytes.Buffer
			var path [4]byte
			var walk func(int, int)
			walk = func(state, depth int) {
				if depth >= e.width {
					t.Fatalf("nonterminal beyond max width")
				}
				for b := 0; b < 256; b++ {
					next := mbcsWideData[e.state+state*256+b]
					if next == 0 {
						continue
					}
					path[depth] = byte(b)
					if next == 1 {
						var rec [5]byte
						rec[0] = byte(depth + 1)
						copy(rec[1:], path[:depth+1])
						queries.Write(rec[:])
					} else {
						walk(int(next)-2, depth+1)
					}
				}
			}
			walk(0, 0)
			// Also compare mixed and malformed sequences, including inputs
			// containing multiple characters rather than one state-table path.
			seed := uint32(0x91e10da5)
			for sample := 0; sample < 20000; sample++ {
				seed ^= seed << 13
				seed ^= seed >> 17
				seed ^= seed << 5
				var q [5]byte
				q[0] = byte(1 + seed%4)
				for j := 1; j <= int(q[0]); j++ {
					seed ^= seed << 13
					seed ^= seed >> 17
					seed ^= seed << 5
					q[j] = byte(seed)
				}
				queries.Write(q[:])
			}
			decodeCount := queries.Len() / 5
			var rec [5]byte
			for r := 0; r <= utf8.MaxRune; r++ {
				if r >= 0xd800 && r <= 0xdfff {
					continue
				}
				rec = [5]byte{0, byte(r >> 24), byte(r >> 16), byte(r >> 8), byte(r)}
				queries.Write(rec[:])
			}
			cmd := exec.Command(exe, dir+"/", name)
			cmd.Stdin = bytes.NewReader(queries.Bytes())
			oracle, err := cmd.Output()
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			count := queries.Len() / 5
			if len(oracle) != count*9 {
				t.Fatalf("oracle length %d expected %d", len(oracle), count*9)
			}
			dst := make([]byte, 16)
			raw := queries.Bytes()
			for i := 0; i < count; i++ {
				req := raw[i*5 : i*5+5]
				answer := oracle[i*9 : i*9+9]
				ok := answer[0] != 0
				var want []byte
				if ok {
					want = answer[1:int(answer[0])]
				}
				if i < decodeCount {
					src := req[1 : 1+int(req[0])]
					n, used, err := codec.Decode(dst, src, true)
					if ok {
						if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
							t.Fatalf("decode %x: %x %d %v; ICU %x", src, dst[:n], used, err, want)
						}
					} else if err == nil {
						t.Fatalf("decode %x accepted but ICU rejected", src)
					}
				} else {
					r := rune(req[1])<<24 | rune(req[2])<<16 | rune(req[3])<<8 | rune(req[4])
					src := []byte(string(r))
					n, used, err := codec.Encode(dst, src, true)
					if ok {
						if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
							t.Fatalf("encode U+%04X: %x %d %v; ICU %x", r, dst[:n], used, err, want)
						}
					} else if err == nil {
						t.Fatalf("encode U+%04X accepted but ICU rejected", r)
					}
				}
			}
		})
	}
}
