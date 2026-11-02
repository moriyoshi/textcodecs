//go:build icu

package textcodecs

import (
	"encoding/binary"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleByteICUDifferential(t *testing.T) {
	dir := t.TempDir()
	inputs, err := filepath.Glob("data/icu/mappings/*.ucm")
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
	exe := filepath.Join(t.TempDir(), "sbcs-oracle")
	args = []string{"-O2", "-o", exe, "testdata/sbcs_oracle.c"}
	args = append(args, strings.Fields(string(flags))...)
	if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	for _, path := range inputs {
		name := strings.TrimSuffix(filepath.Base(path), ".ucm")
		t.Run(name, func(t *testing.T) {
			data, err := exec.Command(exe, dir+"/", name).Output()
			if err != nil {
				t.Fatal(err)
			}
			if len(data) != (256+65536)*4 {
				t.Fatalf("unexpected oracle size %d", len(data))
			}
			c, ok := LookupSingleByte(name)
			if !ok {
				t.Fatal("lookup failed")
			}
			dst := make([]byte, 8)
			for b := 0; b < 256; b++ {
				v := binary.LittleEndian.Uint32(data[b*4:])
				n, used, err := c.Decode(dst, []byte{byte(b)})
				if v == 0 {
					if err != ErrUnmapped || used != 0 {
						t.Fatalf("decode %02x accepted: %d %v", b, used, err)
					}
					continue
				}
				if err != nil || used != 1 || string(dst[:n]) != string(rune(v-1)) {
					t.Fatalf("decode %02x: %x %d %v; want U+%04X", b, dst[:n], used, err, v-1)
				}
			}
			for r := 0; r < 65536; r++ {
				if r >= 0xd800 && r <= 0xdfff {
					continue
				}
				v := binary.LittleEndian.Uint32(data[(256+r)*4:])
				src := []byte(string(rune(r)))
				n, used, err := c.Encode(dst, src, true)
				if v == 0 {
					if err != ErrUnmapped || used != 0 {
						t.Fatalf("encode U+%04X accepted: %x %d %v", r, dst[:n], used, err)
					}
					continue
				}
				if v == 257 {
					if err != nil || used != len(src) || n != 0 {
						t.Fatalf("encode ignored U+%04X: %x %d %v", r, dst[:n], used, err)
					}
					continue
				}
				if err != nil || used != len(src) || n != 1 || dst[0] != byte(v-1) {
					t.Fatalf("encode U+%04X: %x %d %v; want %02x", r, dst[:n], used, err, v-1)
				}
			}
		})
	}
}
