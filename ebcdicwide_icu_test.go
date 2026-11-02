//go:build icu

package textcodecs

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestWideShiftICUDifferential(t *testing.T) {
	dir := t.TempDir()
	inputs, err := filepath.Glob("data/icu/ebcdicwide/*.ucm")
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
	exe := filepath.Join(t.TempDir(), "wide-shift-oracle")
	args = []string{"-O2", "-o", exe, "testdata/ebcdicwide_oracle.c"}
	args = append(args, bytesToFields(flags)...)
	if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	for _, name := range WideShiftNames() {
		t.Run(name, func(t *testing.T) {
			c, ok := OpenWideShift(name)
			if !ok {
				t.Fatal(name)
			}
			var q bytes.Buffer
			for b := 0; b < 256; b++ {
				q.Write([]byte{1, byte(b), 0, 0, 0, 0, 0, 0, 0})
			}
			for v := 0; v < 65536; v++ {
				if c.entry.stateful {
					q.Write([]byte{3, 0x0e, byte(v >> 8), byte(v), 0, 0, 0, 0, 0})
					q.Write([]byte{4, 0x0e, byte(v >> 8), byte(v), 0x0f, 0, 0, 0, 0})
				} else {
					q.Write([]byte{2, byte(v >> 8), byte(v), 0, 0, 0, 0, 0, 0})
				}
			}
			decodeCount := q.Len() / 9
			for r := 0; r <= utf8.MaxRune; r++ {
				if r >= 0xd800 && r <= 0xdfff {
					continue
				}
				q.Write([]byte{0, byte(r >> 24), byte(r >> 16), byte(r >> 8), byte(r), 0, 0, 0, 0})
			}
			for i := 0; i < c.entry.pairCount; i++ {
				p := c.entry.encodePairs + i*10
				r1, r2 := ew32(p), ew32(p+4)
				for _, second := range []uint32{r2, 'A', 0, 0x300} {
					q.Write([]byte{5, byte(r1 >> 24), byte(r1 >> 16), byte(r1 >> 8), byte(r1), byte(second >> 24), byte(second >> 16), byte(second >> 8), byte(second)})
				}
			}
			cmd := exec.Command(exe, dir+"/", name)
			cmd.Stdin = bytes.NewReader(q.Bytes())
			oracle, err := cmd.Output()
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			count := q.Len() / 9
			if len(oracle) != count*17 {
				t.Fatalf("oracle size %d want %d", len(oracle), count*17)
			}
			raw := q.Bytes()
			dst := make([]byte, 32)
			for i := 0; i < count; i++ {
				req := raw[i*9 : i*9+9]
				ans := oracle[i*17 : i*17+17]
				ok := ans[0] != 0
				var want []byte
				if ok {
					want = ans[1:int(ans[0])]
				}
				c.Reset()
				if i < decodeCount {
					src := req[1 : 1+int(req[0])]
					n, used, err := c.Decode(dst, src, true)
					if ok {
						if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
							t.Fatalf("decode %x: %x %d %v; ICU %x", src, dst[:n], used, err, want)
						}
					} else if err == nil {
						t.Fatalf("decode %x accepted but ICU rejected", src)
					}
				} else {
					r1 := rune(req[1])<<24 | rune(req[2])<<16 | rune(req[3])<<8 | rune(req[4])
					src := []byte(string(r1))
					if req[0] == 5 {
						r2 := rune(req[5])<<24 | rune(req[6])<<16 | rune(req[7])<<8 | rune(req[8])
						src = append(src, []byte(string(r2))...)
					}
					n, used, err := c.Encode(dst, src, true)
					if ok {
						if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
							t.Fatalf("encode %x: %x %d %v; ICU %x", src, dst[:n], used, err, want)
						}
					} else if err == nil {
						t.Fatalf("encode %x accepted but ICU rejected", src)
					}
				}
			}
		})
	}
}
