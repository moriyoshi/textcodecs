//go:build icu

package textcodecs

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGSMICUDifferential(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("makeconv", "-d", dir, "data/icu/gsm-03.38-2009.ucm").CombinedOutput(); err != nil {
		t.Fatalf("makeconv: %v: %s", err, out)
	}
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "icu-uc").Output()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "gsm-oracle")
	args := append([]string{"-O2", "-o", exe, "testdata/icu_oracle.c"}, bytesToFields(flags)...)
	if out, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v: %s", err, out)
	}
	data, err := exec.Command(exe, dir+"/", "gsm-03.38-2009").Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != (256+65536+65536)*9 {
		t.Fatalf("oracle length %d", len(data))
	}
	i := 0
	next := func() (bool, []byte) {
		rec := data[i : i+9]
		i += 9
		if rec[0] == 0 {
			return false, nil
		}
		return true, rec[1:int(rec[0])]
	}
	dst := make([]byte, 16)
	for size := 1; size <= 2; size++ {
		max := 256
		if size == 2 {
			max = 65536
		}
		for v := 0; v < max; v++ {
			src := []byte{byte(v)}
			if size == 2 {
				src = []byte{byte(v >> 8), byte(v)}
			}
			ok, want := next()
			n, used, err := GSM.Decode(dst, src, true)
			if ok {
				if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
					t.Fatalf("decode %x: %x %d %v; ICU %x", src, dst[:n], used, err, want)
				}
			} else if err == nil {
				t.Fatalf("decode %x accepted but ICU rejected", src)
			}
		}
	}
	for r := 0; r < 65536; r++ {
		ok, want := next()
		if r >= 0xd800 && r <= 0xdfff {
			continue
		}
		src := []byte(string(rune(r)))
		n, used, err := GSM.Encode(dst, src, true)
		if ok {
			if err != nil || used != len(src) || !bytes.Equal(dst[:n], want) {
				t.Fatalf("encode U+%04X: %x %d %v; ICU %x", r, dst[:n], used, err, want)
			}
		} else if err == nil {
			t.Fatalf("encode U+%04X accepted but ICU rejected", r)
		}
	}
}
