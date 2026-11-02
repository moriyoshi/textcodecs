// phpmbucm generates UCMs from the output of the libmbfl C probe.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type node struct {
	child [256]*node
	final bool
	index int
}
type entry struct {
	uni, code string
	precision int
}

// Merge identical suffix states so the ICU state machine stays compact.
func compact(n *node, intern map[string]*node) *node {
	var key strings.Builder
	if n.final {
		key.WriteByte(1)
	} else {
		key.WriteByte(0)
	}
	for i, child := range n.child {
		if child == nil {
			continue
		}
		n.child[i] = compact(child, intern)
		fmt.Fprintf(&key, "/%02x:%p", i, n.child[i])
	}
	k := key.String()
	if previous := intern[k]; previous != nil {
		return previous
	}
	intern[k] = n
	return n
}

func main() {
	probe := flag.String("probe", "", "path to compiled libmbfl probe")
	out := flag.String("output", "data/php-mbstring", "output directory")
	revision := flag.String("revision", "", "php-src revision used to build the probe")
	flag.Parse()
	if *probe == "" || *revision == "" || flag.NArg() != 0 {
		log.Fatal("usage: phpmbucm -probe executable -revision php-src-revision")
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"ASCII", "ISO-8859-1", "ISO-8859-2", "ISO-8859-3", "ISO-8859-4", "ISO-8859-5", "ISO-8859-6", "ISO-8859-7", "ISO-8859-8", "ISO-8859-9", "ISO-8859-10", "ISO-8859-13", "ISO-8859-14", "ISO-8859-15", "ISO-8859-16", "Windows-1251", "Windows-1252", "Windows-1254", "CP866", "CP850", "KOI8-R", "KOI8-U", "ArmSCII-8", "SJIS", "SJIS-2004", "SJIS-mac", "SJIS-Mobile-DOCOMO", "SJIS-Mobile-KDDI", "SJIS-Mobile-SOFTBANK", "CP932", "SJIS-win", "EUC-JP", "eucJP-win", "CP51932", "EUC-JP-2004", "EUC-CN", "EUC-TW", "EUC-KR", "UHC", "CP936", "BIG5", "CP950"} {
		if err := generate(*probe, *out, *revision, name); err != nil {
			log.Fatal(err)
		}
	}
}
func generate(probe, out, revision, name string) error {
	cmd := exec.Command(probe, name)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	dec := map[string]string{}
	enc := map[string]string{}
	scan := bufio.NewScanner(pipe)
	for scan.Scan() {
		f := strings.Fields(scan.Text())
		if len(f) != 3 {
			return fmt.Errorf("%s: invalid probe output %q", name, scan.Text())
		}
		switch f[0] {
		case "D":
			if v, ok := dec[f[1]]; ok && v != f[2] {
				return fmt.Errorf("duplicate decode %s", f[1])
			}
			dec[f[1]] = f[2]
		case "E":
			if v, ok := enc[f[1]]; ok && v != f[2] {
				return fmt.Errorf("duplicate encode %s", f[1])
			}
			enc[f[1]] = f[2]
		default:
			return fmt.Errorf("invalid record %s", f[0])
		}
	}
	if err = scan.Err(); err != nil {
		return err
	}
	if err = cmd.Wait(); err != nil {
		return err
	}
	root := &node{}
	omitted := 0
	for code := range dec {
		b, err := hex.DecodeString(code)
		if err != nil {
			return err
		}
		p := root
		for _, v := range b {
			if p.final {
				return fmt.Errorf("%s: prefix conflict %s", name, code)
			}
			if p.child[v] == nil {
				p.child[v] = &node{}
			}
			p = p.child[v]
		}
		for _, x := range p.child {
			if x != nil {
				return fmt.Errorf("%s: prefix conflict %s", name, code)
			}
		}
		p.final = true
	}
	// An encoder can emit a byte which is illegal as a complete decoded input
	// (for example SoftBank's escape byte). Preserve it as from-Unicode-only.
	for _, code := range enc {
		if _, ok := dec[code]; ok || len(code) != 2 {
			continue
		}
		v, err := strconv.ParseUint(code, 16, 8)
		if err != nil {
			return err
		}
		if root.child[v] == nil {
			root.child[v] = &node{final: true}
		}
	}
	root = compact(root, map[string]*node{})
	states := []*node{root}
	root.index = 0
	for i := 0; i < len(states); i++ {
		for _, ch := range states[i].child {
			if ch != nil && !ch.final && ch.index == 0 {
				ch.index = len(states)
				states = append(states, ch)
			}
		}
	}
	var entries []entry
	for code, uni := range dec {
		precision := 3
		if enc[uni] == code {
			precision = 0
		}
		entries = append(entries, entry{uni, code, precision})
	}
	for uni, code := range enc {
		if _, ok := dec[code]; !ok && (len(code) != 2 || root.child[mustByte(code)] == nil || !root.child[mustByte(code)].final) {
			omitted++
			fmt.Fprintf(os.Stderr, "%s: omitted Unicode %s -> %s (output is not a single decodable character)\n", name, uni, code)
			continue
		}
		if dec[code] != uni {
			entries = append(entries, entry{uni, code, 4})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.uni != b.uni {
			return a.uni < b.uni
		}
		if a.precision != b.precision {
			return a.precision < b.precision
		}
		return a.code < b.code
	})
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Generated from PHP mbstring libmbfl conversion functions.\n# php-src revision: %s\n# PHP name: %s\n# Contains stateless single-character mappings only; see README.md.\n", revision, name)
	class := "MBCS"
	if maxWidth(dec) == 1 {
		class = "SBCS"
	}
	fmt.Fprintf(&b, "<code_set_name> \"php-%s\"\n<char_name_mask> \"AXXXX\"\n<mb_cur_min> 1\n<mb_cur_max> %d\n<uconv_class> \"%s\"\n<subchar> ", strings.ToLower(name), maxWidth(dec), class)
	sub := "3F"
	if _, ok := dec[sub]; !ok {
		return fmt.Errorf("%s has no question mark", name)
	}
	fmt.Fprint(&b, ucmBytes(sub), "\n")
	for _, state := range states {
		if class == "SBCS" {
			break
		}
		fmt.Fprint(&b, "<icu:state> ")
		first := true
		for i := 0; i < 256; i++ {
			ch := state.child[i]
			if ch == nil {
				continue
			}
			j := i
			for j+1 < 256 && state.child[j+1] == ch {
				j++
			}
			if !first {
				fmt.Fprint(&b, ", ")
			}
			first = false
			fmt.Fprintf(&b, "%02x", i)
			if j > i {
				fmt.Fprintf(&b, "-%02x", j)
			}
			if ch.final {
				fmt.Fprint(&b, ".p")
			} else {
				fmt.Fprintf(&b, ":%x", ch.index)
			}
			i = j
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "CHARMAP")
	for _, e := range entries {
		for i := 0; i < len(e.uni); i += 8 {
			v, err := strconv.ParseUint(e.uni[i:i+8], 16, 32)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "<U%04X>", v)
		}
		fmt.Fprintf(&b, " %s |%d\n", ucmBytes(e.code), e.precision)
	}
	fmt.Fprintln(&b, "END CHARMAP")
	path := filepath.Join(out, "php-"+strings.ToLower(name)+".ucm")
	if err = os.WriteFile(path, b.Bytes(), 0644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s: %d decode, %d encode, %d non-single-character encodings omitted\n", name, len(dec), len(enc), omitted)
	return nil
}
func mustByte(s string) byte {
	n, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		panic(err)
	}
	return byte(n)
}
func maxWidth(m map[string]string) int {
	n := 1
	for k := range m {
		if len(k)/2 > n {
			n = len(k) / 2
		}
	}
	return n
}
func ucmBytes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i += 2 {
		b.WriteString(`\x`)
		b.WriteString(s[i : i+2])
	}
	return b.String()
}
