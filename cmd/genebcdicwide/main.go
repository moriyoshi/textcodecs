// genebcdicwide builds shift-state and DBCS converters with supplementary
// code points and two-scalar Unicode mappings from pinned ICU UCM files.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var mapRE = regexp.MustCompile(`^<U([0-9A-Fa-f]{1,6})>(?:<U([0-9A-Fa-f]{1,6})>)?\s+\\x([0-9A-Fa-f]{2})(?:\\x([0-9A-Fa-f]{2}))?\s+\|([0-4])$`)
var stateRE = regexp.MustCompile(`^([0-9A-Fa-f]{1,2})(?:-([0-9A-Fa-f]{1,2}))?(?::([0-9A-Fa-f]+))?(?:\.([isup]?))?$`)

type pair struct{ first, second rune }
type table struct {
	stateful  bool
	base      string
	states    [][256]byte
	single    [256]uint32
	double    [65536]uint32
	encode    map[rune]uint16
	sequences map[pair]uint16
	decoded   []pair
}

func hex(s string) int {
	v, e := strconv.ParseUint(s, 16, 32)
	if e != nil {
		panic(e)
	}
	return int(v)
}
func row(text string) ([256]byte, error) {
	var out [256]byte
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "initial" {
			continue
		}
		m := stateRE.FindStringSubmatch(part)
		if m == nil {
			return out, fmt.Errorf("unsupported state %q", part)
		}
		lo, hi := hex(m[1]), hex(m[1])
		if m[2] != "" {
			hi = hex(m[2])
		}
		if hi < lo || hi > 255 {
			return out, fmt.Errorf("invalid range %q", part)
		}
		value := byte(1)
		if m[3] != "" && m[4] == "" && !strings.HasSuffix(part, ".") {
			n := hex(m[3])
			if n > 253 {
				return out, fmt.Errorf("state too large")
			}
			value = byte(n + 2)
		}
		if m[4] == "i" || m[4] == "s" {
			value = 0
		}
		if m[3] != "" && m[4] != "" && m[3] != "1" && m[4] != "s" {
			return out, fmt.Errorf("unexpected final-state transition %q", part)
		}
		for i := lo; i <= hi; i++ {
			out[i] = value
		}
	}
	return out, nil
}
func private(r rune) bool {
	return r >= 0xe000 && r <= 0xf8ff || r >= 0xf0000 && r <= 0xffffd || r >= 0x100000 && r <= 0x10fffd
}
func parse(data []byte) (table, error) {
	t := table{encode: map[rune]uint16{}, sequences: map[pair]uint16{}}
	in, done := false, false
	class := ""
	width := 0
	s := bufio.NewScanner(bytes.NewReader(data))
	line := 0
	for s.Scan() {
		line++
		text := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if text == "" {
			continue
		}
		if done {
			if text == "CHARMAP" {
				done = false
				continue
			}
			return t, fmt.Errorf("line %d: extra content", line)
		}
		if !in {
			if text == "CHARMAP" {
				if class == "" || width != 2 || len(t.states) == 0 {
					return t, fmt.Errorf("line %d: missing header", line)
				}
				in = true
				continue
			}
			fields := strings.Fields(text)
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "<icu:state>":
				v, e := row(strings.TrimSpace(strings.TrimPrefix(text, "<icu:state>")))
				if e != nil {
					return t, fmt.Errorf("line %d: %w", line, e)
				}
				t.states = append(t.states, v)
			case "<uconv_class>":
				if len(fields) != 2 || class != "" || (fields[1] != `"EBCDIC_STATEFUL"` && fields[1] != `"DBCS"`) {
					return t, fmt.Errorf("line %d: invalid class", line)
				}
				class = strings.Trim(fields[1], `"`)
				t.stateful = class == "EBCDIC_STATEFUL"
			case "<mb_cur_max>":
				if len(fields) != 2 || fields[1] != "2" || width != 0 {
					return t, fmt.Errorf("line %d: width", line)
				}
				width = 2
			case "<icu:base>":
				if len(fields) != 2 || t.base != "" {
					return t, fmt.Errorf("line %d: base", line)
				}
				var e error
				t.base, e = strconv.Unquote(fields[1])
				if e != nil || t.base == "" {
					return t, fmt.Errorf("line %d: base", line)
				}
			case "<code_set_name>", "<char_name_mask>", "<mb_cur_min>", "<subchar>", "<subchar1>", "<icu:charsetFamily>", "<icu:alias>":
			default:
				return t, fmt.Errorf("line %d: unsupported header %s", line, fields[0])
			}
			continue
		}
		if text == "END CHARMAP" {
			done = true
			continue
		}
		m := mapRE.FindStringSubmatch(text)
		if m == nil {
			return t, fmt.Errorf("line %d: unsupported mapping %q", line, text)
		}
		r := rune(hex(m[1]))
		second := rune(0)
		if m[2] != "" {
			second = rune(hex(m[2]))
			if second == 0 || !utf8.ValidRune(second) {
				return t, fmt.Errorf("line %d: invalid second scalar", line)
			}
		}
		if !utf8.ValidRune(r) {
			return t, fmt.Errorf("line %d: invalid scalar", line)
		}
		a := hex(m[3])
		two := m[4] != ""
		b := 0
		if two {
			b = hex(m[4])
			if a == 0 {
				return t, fmt.Errorf("line %d: zero lead", line)
			}
		}
		initial := 0
		if t.stateful && two {
			initial = 1
		}
		first := t.states[initial][a]
		if two {
			if first < 2 || int(first-2) >= len(t.states) || t.states[first-2][b] != 1 {
				return t, fmt.Errorf("line %d: invalid byte pair", line)
			}
		} else if first != 1 {
			return t, fmt.Errorf("line %d: invalid byte", line)
		}
		code := a
		if two {
			code = a<<8 | b
		}
		if code == 65535 {
			return t, fmt.Errorf("line %d: code overflow", line)
		}
		if m[5] == "0" || m[5] == "3" {
			v := uint32(r) + 1
			if second != 0 {
				v = 0x110001 + uint32(len(t.decoded))
				t.decoded = append(t.decoded, pair{r, second})
			}
			if two {
				if t.double[code] != 0 {
					return t, fmt.Errorf("line %d: duplicate decode", line)
				}
				t.double[code] = v
			} else {
				if t.single[code] != 0 {
					return t, fmt.Errorf("line %d: duplicate decode", line)
				}
				t.single[code] = v
			}
		}
		if m[5] == "0" || m[5] == "4" || (m[5] == "1" && private(r)) {
			if second != 0 {
				p := pair{r, second}
				if _, ok := t.sequences[p]; ok {
					return t, fmt.Errorf("line %d: duplicate sequence", line)
				}
				t.sequences[p] = uint16(code) + 1
			} else {
				if _, ok := t.encode[r]; ok {
					return t, fmt.Errorf("line %d: duplicate scalar", line)
				}
				t.encode[r] = uint16(code) + 1
			}
		}
	}
	if e := s.Err(); e != nil {
		return t, e
	}
	if !done {
		return t, fmt.Errorf("missing END CHARMAP")
	}
	for _, r := range t.states {
		for _, v := range r {
			if v >= 2 && int(v-2) >= len(t.states) {
				return t, fmt.Errorf("invalid state index")
			}
		}
	}
	return t, nil
}
func put16(b *bytes.Buffer, v uint16) {
	if e := binary.Write(b, binary.LittleEndian, v); e != nil {
		log.Fatal(e)
	}
}
func put32(b *bytes.Buffer, v uint32) {
	if e := binary.Write(b, binary.LittleEndian, v); e != nil {
		log.Fatal(e)
	}
}
func main() {
	dir := flag.String("input", "data/icu/ebcdicwide", "UCM directory")
	out := flag.String("output", "data/icu/ebcdicwide.dat", "binary output")
	meta := flag.String("metadata", "ebcdicwide_generated.go", "Go registry")
	manifest := flag.String("manifest", "data/icu/ebcdicwide.sha256", "source hashes")
	flag.Parse()
	files, e := filepath.Glob(filepath.Join(*dir, "*.ucm"))
	if e != nil || len(files) == 0 {
		log.Fatal("no UCM files")
	}
	sort.Strings(files)
	allowed := map[string]string{"ibm-1399_P110-2003": "9d5ed2fddea2b9f491743b0b419f5b87ada1f444fac69c25153ea100905875e4", "ibm-16684_P110-2003": "28bbbbd36646740e4e6ddd89089987a661ec004e894edf5d3c7c7a6ee2be3f0f"}
	var data, g, hashes bytes.Buffer
	ids := map[string]uint16{}
	var blocks [][]uint32
	fmt.Fprintln(&g, "// Code generated by cmd/genebcdicwide; DO NOT EDIT.\npackage textcodecs\n\nvar ebcdicWideEntries = [...]ebcdicWideEntry{")
	for _, path := range files {
		raw, e := os.ReadFile(path)
		if e != nil {
			log.Fatal(e)
		}
		t, e := parse(raw)
		if e != nil {
			log.Fatalf("%s: %v", path, e)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".ucm")
		if t.base != "" && allowed[name] != fmt.Sprintf("%x", sha256.Sum256(raw)) {
			log.Fatalf("%s: unverified delta", path)
		}
		state := data.Len()
		for _, row := range t.states {
			data.Write(row[:])
		}
		single := data.Len()
		for _, v := range t.single {
			put32(&data, v)
		}
		index := data.Len()
		for block := 0; block < 2048; block++ {
			chunk := t.double[block*32 : (block+1)*32]
			var tmp bytes.Buffer
			nonzero := false
			for _, v := range chunk {
				put32(&tmp, v)
				nonzero = nonzero || v != 0
			}
			var id uint16
			if nonzero {
				key := tmp.String()
				var ok bool
				id, ok = ids[key]
				if !ok {
					if len(blocks) >= 65535 {
						log.Fatal("block overflow")
					}
					id = uint16(len(blocks) + 1)
					ids[key] = id
					blocks = append(blocks, append([]uint32(nil), chunk...))
				}
			}
			put16(&data, id)
		}
		decPairs := data.Len()
		for _, v := range t.decoded {
			put32(&data, uint32(v.first))
			put32(&data, uint32(v.second))
		}
		encode := data.Len()
		rs := make([]int, 0, len(t.encode))
		for r := range t.encode {
			rs = append(rs, int(r))
		}
		sort.Ints(rs)
		for _, r := range rs {
			put32(&data, uint32(r))
			put16(&data, t.encode[rune(r)])
		}
		pairs := make([]pair, 0, len(t.sequences))
		for p := range t.sequences {
			pairs = append(pairs, p)
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].first != pairs[j].first {
				return pairs[i].first < pairs[j].first
			}
			return pairs[i].second < pairs[j].second
		})
		encodePairs := data.Len()
		for _, p := range pairs {
			put32(&data, uint32(p.first))
			put32(&data, uint32(p.second))
			put16(&data, t.sequences[p])
		}
		fmt.Fprintf(&g, "{name:%q,state:%d,single:%d,index:%d,decodePairs:%d,encode:%d,encodeCount:%d,encodePairs:%d,pairCount:%d,stateful:%t},\n", name, state, single, index, decPairs, encode, len(rs), encodePairs, len(pairs), t.stateful)
		fmt.Fprintf(&hashes, "%x  %s\n", sha256.Sum256(raw), filepath.Base(path))
	}
	fmt.Fprintln(&g, "}")
	shared := data.Len()
	for _, block := range blocks {
		for _, v := range block {
			put32(&data, v)
		}
	}
	fmt.Fprintf(&g, "const ebcdicWideBlocks = %d\n", shared)
	code, e := format.Source(g.Bytes())
	if e != nil {
		log.Fatal(e)
	}
	for _, v := range []struct {
		path string
		data []byte
	}{{*out, data.Bytes()}, {*meta, code}, {*manifest, hashes.Bytes()}} {
		if e := os.WriteFile(v.path, v.data, 0644); e != nil {
			log.Fatal(e)
		}
	}
	fmt.Fprintf(os.Stderr, "%d converters, %d table bytes\n", len(files), data.Len())
}
