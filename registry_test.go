package textcodecs

import (
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	names := EncodingNames()
	if len(names) != 230 {
		t.Fatalf("got %d names", len(names))
	}
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			t.Fatalf("duplicate %s", name)
		}
		seen[key] = true
		c, ok := Open(strings.ToUpper(name))
		if !ok || c == nil {
			t.Fatalf("cannot open %s", name)
		}
		c.Reset()
	}
	if c, ok := Open("unknown"); ok || c != nil {
		t.Fatal("unknown encoding accepted")
	}
	for _, name := range []string{"ibm-5348_P100-1997", "ibm-1386_P100-2001", "euc-jp-2007", "ibm-943_P15A-2003"} {
		c, ok := Open(name)
		if !ok {
			t.Fatal(name)
		}
		dst := make([]byte, 8)
		n, used, err := c.Encode(dst, []byte("A"), true)
		if err != nil || n != 1 || used != 1 {
			t.Fatalf("%s: %d %d %v", name, n, used, err)
		}
		c.Reset()
		var out [8]byte
		m, used, err := c.Decode(out[:], dst[:n], true)
		if err != nil || used != 1 || string(out[:m]) != "A" {
			t.Fatalf("%s decode: %d %d %v", name, m, used, err)
		}
	}
}
