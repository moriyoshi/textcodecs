package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGB18030Ranges(t *testing.T) {
	source := `static const uint32_t gb18030Ranges[1][4] = {{0x10000, 0x10009, LINEAR(0x90308130), LINEAR(0x90308139)}};`
	local := "var gb18030Ranges = [...]gb18030Range{\n{0x10000, 0x10009, 0x90308130},\n}\n"
	if err := checkGB18030([]byte(source), []byte(local)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(source, "0x10009", "0x10008", 1), strings.Replace(source, "0x90308139", "0x90308138", 1), "missing"} {
		if err := checkGB18030([]byte(bad), []byte(local)); err == nil {
			t.Fatalf("accepted invalid source %q", bad)
		}
	}
}

func TestApplyRollbackAndConflict(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	paths := []string{"a", "b"}
	for _, p := range paths {
		if err := os.WriteFile(filepath.Join(root, p), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	original, err := takeSnapshot(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "a"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := apply(root, stage, paths, original); err == nil {
		t.Fatal("missing staged file accepted")
	}
	b, err := os.ReadFile(filepath.Join(root, "a"))
	if err != nil || string(b) != "old" {
		t.Fatalf("rollback: %q %v", b, err)
	}
	if err := os.WriteFile(filepath.Join(stage, "b"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := apply(root, stage, paths, original); err == nil {
		t.Fatal("concurrent change accepted")
	}
	b, err = os.ReadFile(filepath.Join(root, "a"))
	if err != nil || string(b) != "old" {
		t.Fatalf("conflict wrote file: %q %v", b, err)
	}
	if err := os.WriteFile(filepath.Join(root, "b"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := apply(root, stage, paths, original); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil || string(b) != "new" {
			t.Fatalf("%s: %q %v", p, b, err)
		}
	}
}

func TestSourceObjectsReadsCommittedData(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	git("init", "-q")
	for path, data := range map[string]string{mappingPath + "/test.ucm": "committed", implementationPath: "implementation"} {
		full := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "fixture")
	if err := os.WriteFile(filepath.Join(repo, mappingPath, "test.ucm"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	commit, files, impl, err := sourceObjects(repo, "HEAD", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(commit) != 40 || string(files["test.ucm"]) != "committed" || string(impl) != "implementation" {
		t.Fatalf("unexpected committed objects: %s %q %q", commit, files["test.ucm"], impl)
	}
	git("config", "uploadpack.allowFilter", "true")
	remoteCommit, remoteFiles, remoteImpl, err := sourceObjects("file://"+repo, "HEAD", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if remoteCommit != commit || string(remoteFiles["test.ucm"]) != "committed" || string(remoteImpl) != "implementation" {
		t.Fatal("remote fetch did not return the committed objects")
	}
}
