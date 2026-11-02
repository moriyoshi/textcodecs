//go:build icu

package textcodecs

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// This optional test builds a separate ICU oracle. It needs ICU 74.2, a C
// compiler and pkg-config; ordinary library builds have no ICU dependency.
func TestICUDifferential(t *testing.T) {
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "icu-uc").Output()
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "oracle")
	args := []string{"-O2", "-o", exe, "testdata/icu_oracle.c"}
	args = append(args, bytesToFields(flags)...)
	if output, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile oracle: %v: %s", err, output)
	}
	data, err := exec.Command(exe).Output()
	if err != nil {
		t.Fatalf("run oracle: %v", err)
	}
	const records = 256 + 65536 + 0x110000
	if len(data) != records*9 {
		t.Fatalf("oracle returned %d bytes", len(data))
	}
	index := 0
	next := func() (bool, []byte) {
		rec := data[index : index+9]
		index += 9
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
		for i := 0; i < max; i++ {
			input := []byte{byte(i)}
			if size == 2 {
				input = []byte{byte(i >> 8), byte(i)}
			}
			ok, want := next()
			n, consumed, err := testCP932.Decode(dst, input, true)
			if ok {
				if err != nil || consumed != size || !bytes.Equal(dst[:n], want) {
					t.Fatalf("decode %x got %x (%d,%v), want %x", input, dst[:n], consumed, err, want)
				}
			} else if err == nil {
				t.Fatalf("decode %x accepted; ICU rejected", input)
			}
		}
	}
	for r := 0; r <= utf8.MaxRune; r++ {
		ok, want := next()
		if r >= 0xd800 && r <= 0xdfff {
			continue
		}
		input := []byte(string(rune(r)))
		n, consumed, err := testCP932.Encode(dst, input, true)
		if ok {
			if err != nil || consumed != len(input) || !bytes.Equal(dst[:n], want) {
				t.Fatalf("encode U+%04X got %x (%d,%v), want %x", r, dst[:n], consumed, err, want)
			}
		} else if err == nil {
			t.Fatalf("encode U+%04X accepted; ICU rejected", r)
		}
	}
}

func bytesToFields(b []byte) []string {
	parts := bytes.Fields(b)
	result := make([]string, len(parts))
	for i, p := range parts {
		result[i] = string(p)
	}
	return result
}
