// syncucm updates the pinned ICU mapping sources and their generated tables.
// Updates are staged and checked before any working-tree files are replaced.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	upstreamURL        = "https://github.com/unicode-org/icu.git"
	mappingPath        = "icu4c/source/data/mappings"
	implementationPath = "icu4c/source/common/ucnvmbcs.cpp"
	inventoryPath      = "data/icu/ucm-inventory.sha256"
	revisionPath       = "data/icu/UPSTREAM_REVISION"
)

var families = []string{"mappings", "mbcs2", "mbcswide", "ebcdic", "ebcdicwide"}
var generated = []string{
	"sbcs_generated.go", "mbcs2_generated.go", "mbcswide_generated.go", "ebcdicwide_generated.go", "gsm_generated.go",
	"data/icu/sbcs.dat", "data/icu/mbcs2.dat", "data/icu/mbcswide.dat", "data/icu/ebcdicwide.dat",
	"data/icu/sbcs.sha256", "data/icu/mbcs2.sha256", "data/icu/mbcswide.sha256", "data/icu/ebcdicwide.sha256",
}

type options struct {
	root, repo, ref string
	check           bool
}
type object struct{ path, id string }

func main() {
	var o options
	flag.StringVar(&o.root, "root", ".", "textcodecs workspace root")
	flag.StringVar(&o.repo, "repo", upstreamURL, "ICU Git URL or local checkout")
	flag.StringVar(&o.ref, "ref", "main", "ICU branch, tag, or commit")
	flag.BoolVar(&o.check, "check", false, "validate and report updates without writing")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	changed, err := syncUCM(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "UCM sync failed:", err)
		os.Exit(2)
	}
	if changed && o.check {
		os.Exit(1)
	}
}

func run(cwd string, env []string, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	cmd.Env = env
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(errout.String())
		if detail == "" {
			detail = strings.TrimSpace(out.String())
		}
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
	}
	return out.Bytes(), nil
}
func digest(data []byte) string { v := sha256.Sum256(data); return hex.EncodeToString(v[:]) }
func readInventory(root string) (map[string]string, error) {
	f, e := os.Open(filepath.Join(root, inventoryPath))
	if e != nil {
		return nil, e
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		parts := strings.Fields(line)
		if len(parts) != 2 || len(parts[0]) != 64 {
			return nil, fmt.Errorf("invalid inventory line %q", line)
		}
		if _, e := hex.DecodeString(parts[0]); e != nil || strings.ToLower(parts[0]) != parts[0] {
			return nil, fmt.Errorf("invalid inventory hash %q", parts[0])
		}
		name := parts[1]
		if filepath.Base(name) != name || !strings.HasSuffix(name, ".ucm") {
			return nil, fmt.Errorf("invalid inventory name %q", name)
		}
		if _, ok := out[name]; ok {
			return nil, fmt.Errorf("duplicate inventory name %q", name)
		}
		out[name] = parts[0]
	}
	return out, s.Err()
}
func localSources(root string, inventory map[string]string) (map[string]string, error) {
	out := map[string]string{}
	dirs := append(append([]string(nil), families...), "")
	for _, dir := range dirs {
		pattern := filepath.Join(root, "data", "icu", dir, "*.ucm")
		files, e := filepath.Glob(pattern)
		if e != nil {
			return nil, e
		}
		for _, path := range files {
			name := filepath.Base(path)
			if _, ok := out[name]; ok {
				return nil, fmt.Errorf("duplicate local UCM name: %s", name)
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return nil, e
			}
			out[name] = rel
		}
	}
	if len(out) != len(inventory) {
		return nil, errors.New("local UCM files do not match recorded inventory; resolve local changes first")
	}
	for name, rel := range out {
		want, ok := inventory[name]
		if !ok {
			return nil, fmt.Errorf("local UCM is not in inventory: %s", rel)
		}
		data, e := os.ReadFile(filepath.Join(root, rel))
		if e != nil {
			return nil, e
		}
		if digest(data) != want {
			return nil, fmt.Errorf("local UCM has unrecorded changes: %s", rel)
		}
	}
	return out, nil
}
func sourceObjects(repo, ref, temp string) (string, map[string][]byte, []byte, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return "", nil, nil, errors.New("invalid Git ref")
	}
	gitdir := repo
	rev := ref
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if st, e := os.Stat(repo); e != nil || !st.IsDir() {
		gitdir = filepath.Join(temp, "icu-source")
		if _, e := run("", env, nil, "git", "init", "-q", gitdir); e != nil {
			return "", nil, nil, e
		}
		if _, e := run("", env, nil, "git", "-C", gitdir, "remote", "add", "origin", repo); e != nil {
			return "", nil, nil, e
		}
		if _, e := run("", env, nil, "git", "-C", gitdir, "fetch", "-q", "--depth=1", "--filter=blob:none", "origin", ref); e != nil {
			return "", nil, nil, e
		}
		rev = "FETCH_HEAD"
	}
	out, e := run("", env, nil, "git", "-C", gitdir, "rev-parse", "--verify", rev+"^{commit}")
	if e != nil {
		return "", nil, nil, e
	}
	commit := strings.TrimSpace(string(out))
	if !regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`).MatchString(commit) {
		return "", nil, nil, errors.New("invalid Git commit ID")
	}
	listing, e := run("", env, nil, "git", "-C", gitdir, "ls-tree", "-rz", commit, "--", mappingPath, implementationPath)
	if e != nil {
		return "", nil, nil, e
	}
	var objects []object
	for _, record := range bytes.Split(listing, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		h, n, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return "", nil, nil, errors.New("invalid git ls-tree output")
		}
		parts := strings.Fields(string(h))
		if len(parts) != 3 {
			return "", nil, nil, errors.New("invalid git ls-tree header")
		}
		name := string(n)
		if name != implementationPath && !(filepath.ToSlash(filepath.Dir(name)) == mappingPath && strings.HasSuffix(name, ".ucm")) {
			continue
		}
		if parts[1] != "blob" || (parts[0] != "100644" && parts[0] != "100755") {
			return "", nil, nil, fmt.Errorf("unsupported source entry %s", name)
		}
		objects = append(objects, object{name, parts[2]})
	}
	var query strings.Builder
	for _, o := range objects {
		query.WriteString(o.id)
		query.WriteByte('\n')
	}
	blob, e := run("", env, []byte(query.String()), "git", "-C", gitdir, "cat-file", "--batch")
	if e != nil {
		return "", nil, nil, e
	}
	reader := bufio.NewReader(bytes.NewReader(blob))
	files := map[string][]byte{}
	var impl []byte
	for _, o := range objects {
		header, e := reader.ReadString('\n')
		if e != nil {
			return "", nil, nil, fmt.Errorf("Git object %s: %w", o.path, e)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != o.id || fields[1] != "blob" {
			return "", nil, nil, fmt.Errorf("cannot read Git object %s", o.path)
		}
		size, e := strconv.ParseInt(fields[2], 10, 64)
		if e != nil || size < 0 || size > 1<<30 {
			return "", nil, nil, fmt.Errorf("invalid Git object size for %s", o.path)
		}
		data := make([]byte, int(size))
		if _, e = io.ReadFull(reader, data); e != nil {
			return "", nil, nil, fmt.Errorf("truncated Git object %s", o.path)
		}
		delimiter, e := reader.ReadByte()
		if e != nil || delimiter != '\n' {
			return "", nil, nil, fmt.Errorf("invalid Git object delimiter for %s", o.path)
		}
		if o.path == implementationPath {
			impl = data
		} else {
			name := filepath.Base(o.path)
			if _, ok := files[name]; ok {
				return "", nil, nil, fmt.Errorf("duplicate upstream UCM name %s", name)
			}
			files[name] = data
		}
	}
	if len(files) == 0 || impl == nil {
		return "", nil, nil, errors.New("ICU source is missing mapping files or converter implementation")
	}
	return commit, files, impl, nil
}

var sourceRangesRE = regexp.MustCompile(`(?s)gb18030Ranges\s*\[\s*\d+\s*\]\s*\[\s*4\s*\]\s*=\s*\{(.*?)\}\s*;`)
var sourceRowRE = regexp.MustCompile(`\{\s*(0x[0-9a-fA-F]+)\s*,\s*(0x[0-9a-fA-F]+)\s*,\s*LINEAR\((0x[0-9a-fA-F]+)\)\s*,\s*LINEAR\((0x[0-9a-fA-F]+)\)\s*\}`)
var localRangesRE = regexp.MustCompile(`(?s)var gb18030Ranges\s*=\s*\[\.\.\.\]gb18030Range\s*\{(.*?)\n\}`)
var localRowRE = regexp.MustCompile(`\{\s*(0x[0-9a-fA-F]+)\s*,\s*(0x[0-9a-fA-F]+)\s*,\s*(0x[0-9a-fA-F]+)\s*\}`)

func linear(code uint64) int64 {
	a, b, c, d := int64((code>>24)&255), int64((code>>16)&255), int64((code>>8)&255), int64(code&255)
	return (((a-0x81)*10+b-0x30)*126+c-0x81)*10 + d - 0x30
}
func checkGB18030(source, local []byte) error {
	sm := sourceRangesRE.FindSubmatch(source)
	lm := localRangesRE.FindSubmatch(local)
	if sm == nil || lm == nil {
		return errors.New("cannot find GB18030 algorithmic ranges; review implementation changes")
	}
	src := sourceRowRE.FindAllSubmatch(sm[1], -1)
	dst := localRowRE.FindAllSubmatch(lm[1], -1)
	if len(src) == 0 || len(src) != len(dst) {
		return errors.New("ICU GB18030 algorithmic ranges differ; review implementation changes")
	}
	for i := range src {
		var vals [4]uint64
		for j := 0; j < 4; j++ {
			v, e := strconv.ParseUint(string(src[i][j+1]), 0, 32)
			if e != nil {
				return e
			}
			vals[j] = v
		}
		for j := 0; j < 3; j++ {
			v, e := strconv.ParseUint(string(dst[i][j+1]), 0, 32)
			if e != nil || v != vals[j] {
				return errors.New("ICU GB18030 algorithmic ranges differ; review implementation changes")
			}
		}
		if vals[1] < vals[0] || linear(vals[3])-linear(vals[2]) != int64(vals[1]-vals[0]) {
			return errors.New("ICU GB18030 ranges are no longer linear")
		}
	}
	return nil
}

func copyWorkspace(root, stage string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return os.MkdirAll(stage, 0755)
		}
		if filepath.Dir(rel) == "." && (d.Name() == ".git" || d.Name() == ".agents" || d.Name() == ".codex" || d.Name() == ".aws") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && d.Name() == "__pycache__" {
			return filepath.SkipDir
		}
		target := filepath.Join(stage, rel)
		info, e := d.Info()
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, e := os.Readlink(path)
			if e != nil {
				return e
			}
			return os.Symlink(link, target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported workspace file: %s", rel)
		}
		src, e := os.Open(path)
		if e != nil {
			return e
		}
		defer src.Close()
		dst, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if e != nil {
			return e
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

type snapshot struct {
	data []byte
	mode fs.FileMode
}

func takeSnapshot(root string, paths []string) (map[string]snapshot, error) {
	out := make(map[string]snapshot, len(paths))
	for _, p := range paths {
		path := filepath.Join(root, p)
		info, e := os.Lstat(path)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("sync target is not a regular file: %s", p)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		out[p] = snapshot{data, info.Mode().Perm()}
	}
	return out, nil
}
func atomicWrite(path string, data []byte, mode fs.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".ucm-sync-*")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Chmod(mode); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(temp, path)
}
func apply(root, stage string, paths []string, original map[string]snapshot) error {
	for _, p := range paths {
		info, e := os.Lstat(filepath.Join(root, p))
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("file changed during validation: %s", p)
		}
		data, e := os.ReadFile(filepath.Join(root, p))
		if e != nil || !bytes.Equal(data, original[p].data) {
			return fmt.Errorf("file changed during validation: %s", p)
		}
	}
	var written []string
	for _, p := range paths {
		target := filepath.Join(root, p)
		next, e := os.ReadFile(filepath.Join(stage, p))
		if e == nil && bytes.Equal(next, original[p].data) {
			continue
		}
		if e == nil {
			current, readErr := os.ReadFile(target)
			if readErr != nil || !bytes.Equal(current, original[p].data) {
				e = fmt.Errorf("file changed during update: %s", p)
			}
		}
		if e == nil {
			e = atomicWrite(target, next, original[p].mode)
		}
		if e != nil {
			var rollback []string
			for i := len(written) - 1; i >= 0; i-- {
				prev := written[i]
				current, readErr := os.ReadFile(filepath.Join(root, prev))
				staged, stageErr := os.ReadFile(filepath.Join(stage, prev))
				if readErr != nil || stageErr != nil || !bytes.Equal(current, staged) {
					rollback = append(rollback, prev+": changed since update")
					continue
				}
				if restoreErr := atomicWrite(filepath.Join(root, prev), original[prev].data, original[prev].mode); restoreErr != nil {
					rollback = append(rollback, prev+": "+restoreErr.Error())
				}
			}
			if len(rollback) > 0 {
				return fmt.Errorf("update failed: %w; rollback also failed: %s", e, strings.Join(rollback, ", "))
			}
			return fmt.Errorf("update failed and was rolled back: %w", e)
		}
		written = append(written, p)
	}
	return nil
}

func syncUCM(o options) (bool, error) {
	root, e := filepath.Abs(o.root)
	if e != nil {
		return false, e
	}
	mod, e := os.ReadFile(filepath.Join(root, "go.mod"))
	if e != nil || !strings.Contains(string(mod), "module textcodecs") {
		return false, fmt.Errorf("%s is not the textcodecs workspace", root)
	}
	inv, e := readInventory(root)
	if e != nil {
		return false, e
	}
	paths, e := localSources(root, inv)
	if e != nil {
		return false, e
	}
	targets := append([]string(nil), generated...)
	targets = append(targets, inventoryPath, revisionPath)
	for _, p := range paths {
		targets = append(targets, p)
	}
	sort.Strings(targets)
	original, e := takeSnapshot(root, targets)
	if e != nil {
		return false, e
	}
	temp, e := os.MkdirTemp("", "textcodecs-ucm-sync-*")
	if e != nil {
		return false, e
	}
	defer os.RemoveAll(temp)
	commit, upstream, impl, e := sourceObjects(o.repo, o.ref, temp)
	if e != nil {
		return false, e
	}
	var added, removed []string
	for n := range upstream {
		if _, ok := paths[n]; !ok {
			added = append(added, n)
		}
	}
	for n := range paths {
		if _, ok := upstream[n]; !ok {
			removed = append(removed, n)
		}
	}
	if len(added) > 0 || len(removed) > 0 {
		sort.Strings(added)
		sort.Strings(removed)
		return false, fmt.Errorf("upstream UCM inventory changed; implement/review changes first (added: %v; removed: %v)", added, removed)
	}
	localImpl, e := os.ReadFile(filepath.Join(root, "gb18030.go"))
	if e != nil {
		return false, e
	}
	if e = checkGB18030(impl, localImpl); e != nil {
		return false, e
	}
	stage := filepath.Join(temp, "workspace")
	if e = copyWorkspace(root, stage); e != nil {
		return false, e
	}
	env := os.Environ()
	if os.Getenv("GOCACHE") == "" {
		env = append(env, "GOCACHE="+filepath.Join(temp, "go-cache"))
	}
	if _, e = run(stage, env, nil, "go", "generate", "./..."); e != nil {
		return false, e
	}
	for _, p := range generated {
		data, e := os.ReadFile(filepath.Join(stage, p))
		if e != nil {
			return false, e
		}
		if !bytes.Equal(data, original[p].data) {
			return false, fmt.Errorf("generated file has local changes or is stale: %s; reconcile it before syncing", p)
		}
	}
	names := make([]string, 0, len(upstream))
	for name := range upstream {
		names = append(names, name)
	}
	sort.Strings(names)
	var manifest strings.Builder
	for _, name := range names {
		if e = os.WriteFile(filepath.Join(stage, paths[name]), upstream[name], 0644); e != nil {
			return false, e
		}
		fmt.Fprintf(&manifest, "%s  %s\n", digest(upstream[name]), name)
	}
	if e = os.WriteFile(filepath.Join(stage, inventoryPath), []byte(manifest.String()), 0644); e != nil {
		return false, e
	}
	if e = os.WriteFile(filepath.Join(stage, revisionPath), []byte(commit+"\n"), 0644); e != nil {
		return false, e
	}
	fmt.Println("Generating tables and running ICU differential tests in a temporary workspace...")
	if _, e = run(stage, env, nil, "go", "generate", "./..."); e != nil {
		return false, e
	}
	if _, e = run(stage, env, nil, "go", "test", "-count=1", "-tags", "icu", "./..."); e != nil {
		return false, e
	}
	var changes []string
	for _, p := range targets {
		data, e := os.ReadFile(filepath.Join(stage, p))
		if e != nil {
			return false, e
		}
		if !bytes.Equal(data, original[p].data) {
			changes = append(changes, p)
		}
	}
	if len(changes) == 0 {
		fmt.Printf("Already in sync with ICU %s.\n", commit)
		return false, nil
	}
	fmt.Printf("Validated ICU %s; %d file(s) would change.\n", commit, len(changes))
	for _, p := range changes {
		if strings.HasSuffix(p, ".ucm") {
			fmt.Println(" ", p)
		}
	}
	if o.check {
		return true, nil
	}
	if e = apply(root, stage, targets, original); e != nil {
		return false, e
	}
	fmt.Println("Updated UCM files, generated tables, inventory and pinned revision.")
	return true, nil
}
