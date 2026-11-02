package textcodecs

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedMappingInventory(t *testing.T) {
	f, err := os.Open("data/icu/ucm-inventory.sha256")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	inventory := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) != 2 {
			t.Fatalf("bad inventory line %q", s.Text())
		}
		if _, ok := inventory[fields[1]]; ok {
			t.Fatalf("duplicate %s", fields[1])
		}
		inventory[fields[1]] = fields[0]
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 190 {
		t.Fatalf("inventory has %d entries", len(inventory))
	}
	kanaData, err := os.ReadFile("data/ebcdic-kana/ucm.sha256")
	if err != nil {
		t.Fatal(err)
	}
	kana := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(kanaData)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || kana[fields[1]] != "" {
			t.Fatalf("bad ebcdic-kana inventory line %q", line)
		}
		kana[fields[1]] = fields[0]
		data, err := os.ReadFile(filepath.Join("data/ebcdic-kana", fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != fields[0] {
			t.Fatalf("%s: sha256 %s, want %s", fields[1], got, fields[0])
		}
	}
	if len(kana) != 9 {
		t.Fatalf("ebcdic-kana inventory has %d entries", len(kana))
	}
	php := map[string]bool{}
	for _, list := range []string{"registry-sbcs.txt", "registry-wide.txt"} {
		data, err := os.ReadFile(filepath.Join("data/php-mbstring", list))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range strings.Fields(string(data)) {
			if php[name] {
				t.Fatalf("duplicate PHP mapping %s", name)
			}
			php[name] = true
		}
	}
	phpHashes, err := os.ReadFile("data/php-mbstring/ucm.sha256")
	if err != nil {
		t.Fatal(err)
	}
	verified := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(phpHashes)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("bad PHP inventory line %q", line)
		}
		if !php[fields[1]] {
			continue
		}
		data, err := os.ReadFile(filepath.Join("data/php-mbstring", fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != fields[0] {
			t.Fatalf("%s: hash %s, want %s", fields[1], got, fields[0])
		}
		verified[fields[1]] = true
	}
	if len(verified) != len(php) {
		t.Fatalf("PHP inventory verifies %d of %d registered mappings", len(verified), len(php))
	}
	registered := EncodingNames()
	if len(registered) != len(inventory)+len(kana)+len(php)+4 {
		t.Fatalf("registry has %d encodings, inventory has %d", len(registered), len(inventory))
	}
	for _, name := range registered {
		_, isICU := inventory[name+".ucm"]
		_, isKana := kana[name+".ucm"]
		if !isICU && !isKana && !php[name+".ucm"] && name != "euc-jis-2004" && name != "shift_jis-2004" && name != "nta-jis-x-0213-euc" && name != "nta-jis-x-0213-sjis" {
			t.Fatalf("registered encoding %s is not in the pinned inventory", name)
		}
		if _, ok := Open(name); !ok {
			t.Fatalf("registered encoding %s cannot be opened", name)
		}
	}
	check := func(path, name string) {
		t.Helper()
		want, ok := inventory[name]
		if !ok {
			t.Fatalf("%s absent from inventory", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
			t.Fatalf("%s: sha256 %s, want %s", name, got, want)
		}
	}
	for _, name := range SingleByteNames() {
		file := name + ".ucm"
		if _, ok := kana[file]; ok || php[file] {
			continue
		}
		check(filepath.Join("data/icu/mappings", file), file)
	}
	for _, name := range MultibyteNames() {
		file := name + ".ucm"
		check(filepath.Join("data/icu/mbcs2", file), file)
	}
	for _, name := range WideMultibyteNames() {
		if name == "euc-jis-2004" || name == "shift_jis-2004" || name == "nta-jis-x-0213-euc" || name == "nta-jis-x-0213-sjis" || php[name+".ucm"] {
			continue
		}
		file := name + ".ucm"
		check(filepath.Join("data/icu/mbcswide", file), file)
	}
	for _, name := range StatefulNames() {
		file := name + ".ucm"
		check(filepath.Join("data/icu/ebcdic", file), file)
	}
	for _, name := range WideShiftNames() {
		file := name + ".ucm"
		check(filepath.Join("data/icu/ebcdicwide", file), file)
	}
	check("data/icu/gsm-03.38-2009.ucm", "gsm-03.38-2009.ucm")
}
