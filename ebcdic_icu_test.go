//go:build icu

package textcodecs

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStatefulICUDifferential(t *testing.T) {
	dir := t.TempDir()
	inputs, err := filepath.Glob("data/icu/ebcdic/*.ucm")
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
	exe := filepath.Join(t.TempDir(), "stateful-oracle")
	args = []string{"-O2", "-o", exe, "testdata/mbcswide_oracle.c"}
	args = append(args, bytesToFields(flags)...)
	if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	for _, name := range StatefulNames() {
		t.Run(name, func(t *testing.T) {
			c, ok := OpenStateful(name)
			if !ok {
				t.Fatal(name)
			}
			var queries bytes.Buffer
			for b := 0; b < 256; b++ {
				queries.Write([]byte{1, byte(b), 0, 0, 0})
			}
			for v := 0; v < 65536; v++ {
				queries.Write([]byte{3, 0x0e, byte(v >> 8), byte(v), 0})
			}
			for v := 0; v < 65536; v++ {
				queries.Write([]byte{4, 0x0e, byte(v >> 8), byte(v), 0x0f})
			}
			decodeCount := queries.Len() / 5
			for r := 0; r < 65536; r++ {
				if r >= 0xd800 && r <= 0xdfff {
					continue
				}
				queries.Write([]byte{0, 0, 0, byte(r >> 8), byte(r)})
			}
			cmd := exec.Command(exe, dir+"/", name)
			cmd.Stdin = bytes.NewReader(queries.Bytes())
			oracle, err := cmd.Output()
			if err != nil {
				t.Fatalf("oracle: %v", err)
			}
			count := queries.Len() / 5
			if len(oracle) != count*9 {
				t.Fatalf("oracle size %d want %d", len(oracle), count*9)
			}
			raw := queries.Bytes()
			dst := make([]byte, 16)
			for i := 0; i < count; i++ {
				req := raw[i*5 : i*5+5]
				ans := oracle[i*9 : i*9+9]
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
					r := rune(req[3])<<8 | rune(req[4])
					src := []byte(string(r))
					n, used, err := c.Encode(dst, src, true)
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
