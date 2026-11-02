package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMappingDirections(t *testing.T) {
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe")
	script := "#!/bin/sh\ncat <<'EOF'\nD 3F 0000003F\nD 81 00003042\nD 82 00003042\nD 8140 0000304B0000309A\nE 0000003F 3F\nE 00003042 81\nE 00003043 81\nE 0000304B0000309A 8140\nEOF\n"
	// This fixture intentionally has an ambiguous byte prefix, which cannot
	// be represented by a stateless UCM state machine.
	if err := os.WriteFile(probe, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := generate(probe, dir, "test", "test"); err == nil || !strings.Contains(err.Error(), "prefix conflict") {
		t.Fatalf("got %v", err)
	}
	script = strings.ReplaceAll(script, "D 81 00003042", "D 83 00003042")
	script = strings.ReplaceAll(script, " 81\n", " 83\n")
	if err := os.WriteFile(probe, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := generate(probe, dir, "test", "test"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "php-test.ucm"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`<U3042> \x83 |0`, `<U3042> \x82 |3`, `<U3043> \x83 |4`, `<U304B><U309A> \x81\x40 |0`} {
		if !strings.Contains(string(raw), line) {
			t.Errorf("missing %s", line)
		}
	}
}
