// genmbcswide generates stateless ICU MBCS tables for scalar mappings of one
// to four bytes, including supplementary Unicode characters.
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
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var mapRE = regexp.MustCompile(`^<U([0-9A-Fa-f]{1,6})>((?:<U[0-9A-Fa-f]{1,6}>){0,4})\s+((?:\\x[0-9A-Fa-f]{2}){1,4})\s+\|([0-4])$`)
var sequenceRE = regexp.MustCompile(`<U([0-9A-Fa-f]{1,6})>`)
var rowRE = regexp.MustCompile(`^([0-9A-Fa-f]{1,2})(?:-([0-9A-Fa-f]{1,2}))?(?::([0-9A-Fa-f]+))?(?:\.([iup.]))?$`)

type encoded struct {
	key    uint32
	length byte
}
type unicodePair struct {
	first, second rune
	extra         [3]rune
	code          encoded
}

func (p unicodePair) scalars() [5]rune {
	return [5]rune{p.first, p.second, p.extra[0], p.extra[1], p.extra[2]}
}

type table struct {
	base   string
	states [][256]byte
	decode map[uint32]rune
	encode map[rune]encoded
	ascii  bool
	width  int
	pairs  []unicodePair
}

// parseX0213 reads a Project X0213 mapping, including mappings to two scalars.
func parseX0213(data []byte, euc bool) (table, error) {
	t := table{decode: map[uint32]rune{}, encode: map[rune]encoded{}, ascii: true, width: 2}
	rows := []string{"0-7f, 81-9f:1, a1-df, e0-fc:1", "40-7e, 80-fc"}
	if euc {
		t.width = 3
		rows = []string{"0-8d, 8e:2, 8f:3, 90-9f, a1-fe:1", "a1-fe", "a1-df", "a1-fe:4", "a1-fe"}
	}
	for _, text := range rows {
		row, err := parseState(text)
		if err != nil {
			return t, err
		}
		t.states = append(t.states, row)
	}
	seenPairs := map[[2]rune]bool{}
	s := bufio.NewScanner(bytes.NewReader(data))
	for line := 1; s.Scan(); line++ {
		fields := strings.Fields(s.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "0x") {
			return t, fmt.Errorf("line %d: invalid code", line)
		}
		if strings.HasPrefix(fields[1], "#") { // reserved entry
			continue
		}
		if !strings.HasPrefix(fields[1], "U+") {
			return t, fmt.Errorf("line %d: invalid Unicode mapping", line)
		}
		hexCode := strings.TrimPrefix(fields[0], "0x")
		if len(hexCode) != 2 && len(hexCode) != 4 && len(hexCode) != 6 {
			return t, fmt.Errorf("line %d: invalid code width", line)
		}
		n, err := strconv.ParseUint(hexCode, 16, 24)
		if err != nil {
			return t, fmt.Errorf("line %d: %w", line, err)
		}
		length := len(hexCode) / 2
		if length > t.width {
			return t, fmt.Errorf("line %d: excessive code width", line)
		}
		key := uint32(n) << uint(32-8*length)
		if !(euc && length == 1 && (n == 0x8e || n == 0x8f)) {
			state := 0
			for i := 0; i < length; i++ {
				b := byte(key >> uint(24-8*i))
				next := t.states[state][b]
				if i+1 == length {
					if next != 1 {
						return t, fmt.Errorf("line %d: invalid final byte", line)
					}
				} else {
					if next < 2 {
						return t, fmt.Errorf("line %d: invalid transition", line)
					}
					state = int(next) - 2
				}
			}
		}
		if _, ok := t.decode[key]; ok {
			return t, fmt.Errorf("line %d: duplicate code", line)
		}
		parts := strings.Split(strings.TrimPrefix(fields[1], "U+"), "+")
		if len(parts) < 1 || len(parts) > 2 {
			return t, fmt.Errorf("line %d: unsupported Unicode sequence", line)
		}
		var scalars [2]rune
		for i, part := range parts {
			v, err := strconv.ParseUint(part, 16, 32)
			if err != nil || !utf8.ValidRune(rune(v)) {
				return t, fmt.Errorf("line %d: invalid Unicode scalar", line)
			}
			scalars[i] = rune(v)
		}
		code := encoded{key, byte(length)}
		if len(parts) == 2 {
			if scalars[0] < 0x80 || scalars[1] == 0 || seenPairs[scalars] {
				return t, fmt.Errorf("line %d: unsupported Unicode pair", line)
			}
			seenPairs[scalars] = true
			t.decode[key] = rune(0x110000 + len(t.pairs))
			t.pairs = append(t.pairs, unicodePair{first: scalars[0], second: scalars[1], code: code})
		} else {
			if _, ok := t.encode[scalars[0]]; ok {
				return t, fmt.Errorf("line %d: duplicate Unicode scalar", line)
			}
			t.decode[key] = scalars[0]
			t.encode[scalars[0]] = code
		}
	}
	if err := s.Err(); err != nil {
		return t, err
	}
	if len(t.decode) == 0 || len(t.pairs) == 0 {
		return t, fmt.Errorf("incomplete Project X0213 mapping")
	}
	for b := uint32(0x20); b <= 0x7e; b++ {
		if r, ok := t.decode[b<<24]; !ok || r != rune(b) {
			t.ascii = false
			break
		}
	}
	return t, nil
}

func parseHex(s string) uint32 {
	v, e := strconv.ParseUint(s, 16, 32)
	if e != nil {
		panic(e)
	}
	return uint32(v)
}
func parseState(text string) ([256]byte, error) {
	var row [256]byte
	for _, entry := range strings.Split(text, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		m := rowRE.FindStringSubmatch(entry)
		if m == nil {
			return row, fmt.Errorf("unsupported state %q", entry)
		}
		lo, hi := parseHex(m[1]), parseHex(m[1])
		if m[2] != "" {
			hi = parseHex(m[2])
		}
		if hi < lo || hi > 255 {
			return row, fmt.Errorf("invalid range %q", entry)
		}
		v := byte(1)
		if m[3] != "" && m[4] == "" {
			n := parseHex(m[3])
			if n > 253 {
				return row, fmt.Errorf("invalid state %q", entry)
			}
			v = byte(n + 2)
		}
		if m[4] == "i" {
			v = 0
		}
		if m[3] != "" && m[4] != "" {
			return row, fmt.Errorf("stateful final %q", entry)
		}
		for i := lo; i <= hi; i++ {
			row[i] = v
		}
	}
	return row, nil
}
func private(r rune) bool {
	return r >= 0xe000 && r <= 0xf8ff || r >= 0xf0000 && r <= 0xffffd || r >= 0x100000 && r <= 0x10fffd
}
func parse(data []byte) (table, error) {
	t := table{decode: map[uint32]rune{}, encode: map[rune]encoded{}}
	in, done, cls := false, false, false
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
			return t, fmt.Errorf("line %d: content after map", line)
		}
		if !in {
			if text == "CHARMAP" {
				if !cls || t.width < 1 || len(t.states) == 0 {
					return t, fmt.Errorf("line %d: missing header", line)
				}
				in = true
				continue
			}
			parts := strings.Fields(text)
			if len(parts) == 0 {
				continue
			}
			switch parts[0] {
			case "<icu:state>":
				row, e := parseState(strings.TrimSpace(strings.TrimPrefix(text, "<icu:state>")))
				if e != nil {
					return t, fmt.Errorf("line %d: %w", line, e)
				}
				t.states = append(t.states, row)
			case "<uconv_class>":
				if len(parts) != 2 || parts[1] != `"MBCS"` || cls {
					return t, fmt.Errorf("line %d: class", line)
				}
				cls = true
			case "<mb_cur_max>":
				if len(parts) != 2 || t.width != 0 {
					return t, fmt.Errorf("line %d: width", line)
				}
				n, e := strconv.Atoi(parts[1])
				if e != nil || n < 1 || n > 4 {
					return t, fmt.Errorf("line %d: width", line)
				}
				t.width = n
			case "<icu:base>":
				if len(parts) != 2 || t.base != "" {
					return t, fmt.Errorf("line %d: invalid base", line)
				}
				var err error
				t.base, err = strconv.Unquote(parts[1])
				if err != nil || t.base == "" {
					return t, fmt.Errorf("line %d: invalid base", line)
				}
			case "<code_set_name>", "<char_name_mask>", "<mb_cur_min>", "<subchar>", "<subchar1>", "<icu:charsetFamily>", "<icu:alias>":
			default:
				return t, fmt.Errorf("line %d: unsupported header %s", line, parts[0])
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
		r := rune(parseHex(m[1]))
		if !utf8.ValidRune(r) {
			return t, fmt.Errorf("line %d: invalid scalar", line)
		}
		length := len(m[3]) / 4
		if length > t.width {
			return t, fmt.Errorf("line %d: excessive byte sequence", line)
		}
		state := 0
		var key uint32
		for i := 0; i < length; i++ {
			b := byte(parseHex(m[3][i*4+2 : i*4+4]))
			key |= uint32(b) << uint(24-8*i)
			if state >= len(t.states) {
				return t, fmt.Errorf("line %d: bad state", line)
			}
			next := t.states[state][b]
			if i+1 < length {
				if next < 2 {
					return t, fmt.Errorf("line %d: invalid transition", line)
				}
				state = int(next) - 2
			} else if next != 1 {
				return t, fmt.Errorf("line %d: invalid final byte", line)
			}
		}
		if m[2] != "" {
			sequence := [5]rune{r}
			for i, field := range sequenceRE.FindAllStringSubmatch(m[2], -1) {
				sequence[i+1] = rune(parseHex(field[1]))
				if sequence[i+1] == 0 || !utf8.ValidRune(sequence[i+1]) {
					return t, fmt.Errorf("line %d: invalid sequence scalar", line)
				}
			}
			if m[4] != "0" {
				return t, fmt.Errorf("line %d: unsupported Unicode pair", line)
			}
			if _, ok := t.decode[key]; ok {
				return t, fmt.Errorf("line %d: duplicate decode key", line)
			}
			for _, pair := range t.pairs {
				if pair.scalars() == sequence {
					return t, fmt.Errorf("line %d: duplicate Unicode pair", line)
				}
			}
			t.decode[key] = rune(0x110000 + len(t.pairs))
			t.pairs = append(t.pairs, unicodePair{first: r, second: sequence[1], extra: [3]rune{sequence[2], sequence[3], sequence[4]}, code: encoded{key, byte(length)}})
			continue
		}
		// ICU's MBCS decoder reserves U+FFFE and U+FFFF as table sentinels.
		// The compound-text UCM lists them, but makeconv's decoder reports
		// their byte sequences as unmapped; encoding still uses the entries.
		if (m[4] == "0" || m[4] == "3") && r != 0xfffe && r != 0xffff {
			if _, ok := t.decode[key]; ok {
				return t, fmt.Errorf("line %d: duplicate decode key", line)
			}
			t.decode[key] = r
		}
		if m[4] == "0" || m[4] == "4" || (m[4] == "1" && private(r)) {
			if _, ok := t.encode[r]; ok {
				return t, fmt.Errorf("line %d: duplicate encode", line)
			}
			t.encode[r] = encoded{key, byte(length)}
		}
	}
	if e := s.Err(); e != nil {
		return t, e
	}
	if !done {
		return t, fmt.Errorf("missing END CHARMAP")
	}
	for _, row := range t.states {
		for _, v := range row {
			if v >= 2 && int(v-2) >= len(t.states) {
				return t, fmt.Errorf("missing state")
			}
		}
	}
	visited := make(map[[2]int]bool)
	var checkDepth func(state, depth int) error
	checkDepth = func(state, depth int) error {
		key := [2]int{state, depth}
		if visited[key] {
			return nil
		}
		visited[key] = true
		for _, v := range t.states[state] {
			if v >= 2 {
				if depth+1 >= t.width {
					return fmt.Errorf("state table exceeds declared byte width")
				}
				if err := checkDepth(int(v-2), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := checkDepth(0, 0); err != nil {
		return t, err
	}
	t.ascii = true
	for i := 0; i < 128; i++ {
		r, ok := t.decode[uint32(i)<<24]
		e, ok2 := t.encode[rune(i)]
		if !ok || r != rune(i) || !ok2 || e.key != uint32(i)<<24 || e.length != 1 || t.states[0][i] != 1 {
			t.ascii = false
			break
		}
	}
	for _, pair := range t.pairs {
		if pair.first < 128 {
			t.ascii = false
		}
	}
	return t, nil
}
func write32(b *bytes.Buffer, v uint32) {
	if e := binary.Write(b, binary.LittleEndian, v); e != nil {
		log.Fatal(e)
	}
}
func write64(b *bytes.Buffer, v uint64) {
	if e := binary.Write(b, binary.LittleEndian, v); e != nil {
		log.Fatal(e)
	}
}
func write16(b *bytes.Buffer, v uint16) {
	if e := binary.Write(b, binary.LittleEndian, v); e != nil {
		log.Fatal(e)
	}
}
func write24(b *bytes.Buffer, v uint32) {
	b.WriteByte(byte(v))
	b.WriteByte(byte(v >> 8))
	b.WriteByte(byte(v >> 16))
}

func writeBitmap(b *bytes.Buffer, bitmap [32]byte) {
	b.Write(bitmap[:])
	// A word's prefix is at most 192, so it fits in one byte even for a
	// completely full bitmap. It avoids counting preceding words at lookup.
	count := 0
	for word := 0; word < 4; word++ {
		b.WriteByte(byte(count))
		for _, v := range bitmap[word*8 : word*8+8] {
			count += bits.OnesCount8(v)
		}
	}
}
func main() {
	dir := flag.String("input", "data/icu/mbcswide", "UCM directory")
	out := flag.String("output", "data/icu/mbcswide.dat", "binary output")
	meta := flag.String("metadata", "mbcswide_generated.go", "Go registry")
	manifest := flag.String("manifest", "data/icu/mbcswide.sha256", "source hashes")
	project := flag.String("x0213", "data/x0213/euc-jis-2004-std.txt", "Project X0213 EUC mapping (empty to omit)")
	projectSJIS := flag.String("x0213-sjis", "data/x0213/sjis-0213-2004-std.txt", "Project X0213 Shift_JIS mapping (empty to omit)")
	nta := flag.String("nta", "data/nta/nta-jis-x-0213-euc.ucm", "NTA mapping (empty to omit)")
	ntaSJIS := flag.String("nta-sjis", "data/nta/nta-jis-x-0213-sjis.ucm", "NTA Shift_JIS mapping (empty to omit)")
	phpList := flag.String("php-list", "data/php-mbstring/registry-wide.txt", "selected PHP mappings (empty to omit)")
	flag.Parse()
	files, e := filepath.Glob(filepath.Join(*dir, "*.ucm"))
	if e != nil || len(files) == 0 {
		log.Fatal("no input files")
	}
	if *nta != "" {
		files = append(files, *nta)
	}
	if *ntaSJIS != "" {
		files = append(files, *ntaSJIS)
	}
	phpFiles := map[string]bool{}
	if *phpList != "" {
		list, err := os.ReadFile(*phpList)
		if err != nil {
			log.Fatal(err)
		}
		for _, name := range strings.Fields(string(list)) {
			if filepath.Base(name) != name || !strings.HasSuffix(name, ".ucm") {
				log.Fatalf("invalid PHP mapping %q", name)
			}
			path := filepath.Join(filepath.Dir(*phpList), name)
			phpFiles[path] = true
			files = append(files, path)
		}
	}
	type projectMapping struct {
		name string
		euc  bool
	}
	projectPaths := map[string]projectMapping{}
	for _, source := range []struct {
		path string
		projectMapping
	}{{*project, projectMapping{"euc-jis-2004", true}}, {*projectSJIS, projectMapping{"shift_jis-2004", false}}} {
		if source.path == "" {
			continue
		}
		if _, exists := projectPaths[source.path]; exists {
			log.Fatal("Project X0213 mapping paths must be distinct")
		}
		projectPaths[source.path] = source.projectMapping
		files = append(files, source.path)
	}
	sort.Slice(files, func(i, j int) bool {
		name := func(path string) string {
			if project, ok := projectPaths[path]; ok {
				return project.name
			}
			return strings.TrimSuffix(filepath.Base(path), ".ucm")
		}
		return strings.ToLower(name(files[i])) < strings.ToLower(name(files[j]))
	})
	var data, g, hashes bytes.Buffer
	decIDs, encIDs := map[string]uint16{}, map[string]uint16{}
	var decGroups, encGroups [][]byte
	shortIDs := map[string]uint16{}
	var shortBlocks [][]byte
	bmpIDs := map[string]uint16{}
	var bmpBlocks [][]byte
	var bmpIndexes []int
	intern := func(ids map[string]uint16, groups *[][]byte, raw []byte) uint16 {
		if len(raw) == 0 {
			return 0
		}
		key := string(raw)
		if id, ok := ids[key]; ok {
			return id
		}
		if len(*groups) >= 65535 {
			log.Fatal("group index overflow")
		}
		id := uint16(len(*groups) + 1)
		ids[key] = id
		*groups = append(*groups, append([]byte(nil), raw...))
		return id
	}
	fmt.Fprintln(&g, "// Code generated by cmd/genmbcswide; DO NOT EDIT.\npackage textcodecs\n\nvar mbcsWideEntries = [...]mbcsWideEntry{")
	for _, p := range files {
		raw, e := os.ReadFile(p)
		if e != nil {
			log.Fatal(e)
		}
		var t table
		project, isProject := projectPaths[p]
		if isProject {
			t, e = parseX0213(raw, project.euc)
		} else {
			t, e = parse(raw)
		}
		if e != nil {
			log.Fatalf("%s: %v", p, e)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".ucm")
		if isProject {
			name = project.name
		}
		gb18030 := name == "gb18030-2022"
		if gb18030 && fmt.Sprintf("%x", sha256.Sum256(raw)) != "50b16dfde5438f4c63ff59adcf3a5a6d8d3e6987f2b7c31e4383aaa5a6f6b6e8" {
			log.Fatalf("%s: unexpected GB18030 mapping version", p)
		}
		// This pinned delta contains complete mappings, checked against ICU
		// with its base. Other deltas require an actual base-merging engine.
		if t.base != "" && (name != "ibm-33722_P120-1999" || fmt.Sprintf("%x", sha256.Sum256(raw)) != "a1c2b64f1c5d7f1243fd71fe222310b2c939eadf040deab69cf5e01b81875ad2") {
			log.Fatalf("%s: inherited mapping not verified for direct generation", p)
		}
		state := data.Len()
		for _, row := range t.states {
			data.Write(row[:])
		}
		keys := make([]uint32, 0, len(t.decode))
		for k := range t.decode {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		// Use the same 32-entry, shared-block scheme as the two-byte codec.
		// Wide values occupy three bytes so supplementary scalars fit.
		short := make([]uint32, 1<<16)
		for _, key := range keys {
			if key&0xffff == 0 {
				short[key>>16] = uint32(t.decode[key]) + 1
			}
		}
		shortIndex := data.Len()
		for block := 0; block < len(short)/32; block++ {
			var packed bytes.Buffer
			nonzero := false
			for _, value := range short[block*32 : (block+1)*32] {
				write24(&packed, value)
				nonzero = nonzero || value != 0
			}
			var id uint16
			if nonzero {
				id = intern(shortIDs, &shortBlocks, packed.Bytes())
			}
			write16(&data, id)
		}
		var dIndex [256]uint16
		for i := 0; i < len(keys); {
			group := byte(keys[i] >> 24)
			var long bytes.Buffer
			for i < len(keys) && byte(keys[i]>>24) == group {
				key := keys[i]
				if key&0xffff != 0 {
					write24(&long, key&0xffffff)
					write24(&long, uint32(t.decode[key])+1)
				}
				i++
			}
			dIndex[group] = intern(decIDs, &decGroups, long.Bytes())
		}
		dec := data.Len()
		for _, v := range dIndex {
			write16(&data, v)
		}
		// BMP encoding uses 64-scalar bitmap blocks, as in the two-byte
		// codec. Identical packed blocks are shared across converters.
		bmpIndex := data.Len()
		bmpIndexes = append(bmpIndexes, bmpIndex)
		for base := 0; base < 0x10000; base += 64 {
			var mask uint64
			var values bytes.Buffer
			for bit := 0; bit < 64; bit++ {
				if v, ok := t.encode[rune(base+bit)]; ok {
					mask |= uint64(1) << uint(bit)
					write32(&values, v.key)
					values.WriteByte(v.length)
				}
			}
			var id uint16
			if mask != 0 {
				var raw bytes.Buffer
				write64(&raw, mask)
				raw.Write(values.Bytes())
				id = intern(bmpIDs, &bmpBlocks, raw.Bytes())
			}
			write32(&data, uint32(id))
		}
		runes := make([]int, 0, len(t.encode))
		for r := range t.encode {
			if r >= 0x10000 {
				runes = append(runes, int(r))
			}
		}
		sort.Ints(runes)
		var pages [17][256]uint16
		var present [17]bool
		for i := 0; i < len(runes); {
			high := uint16(runes[i] >> 8)
			// Each Unicode page stores a bitmap followed by five-byte
			// key/length records in increasing low-byte order.
			var bitmap [32]byte
			var values bytes.Buffer
			for i < len(runes) && uint16(runes[i]>>8) == high {
				v := t.encode[rune(runes[i])]
				low := byte(runes[i])
				bitmap[low/8] |= 1 << (low % 8)
				write32(&values, v.key)
				values.WriteByte(v.length)
				i++
			}
			var raw bytes.Buffer
			writeBitmap(&raw, bitmap)
			raw.Write(values.Bytes())
			id := intern(encIDs, &encGroups, raw.Bytes())
			pages[high>>8][byte(high)] = id
			present[high>>8] = true
		}
		enc := data.Len()
		nextPage := uint16(0)
		for _, exists := range present {
			if exists {
				nextPage++
				write16(&data, nextPage)
			} else {
				write16(&data, 0)
			}
		}
		pageData := data.Len()
		for plane, exists := range present {
			if exists {
				for _, id := range pages[plane] {
					write16(&data, id)
				}
			}
		}
		pairDecode := data.Len()
		for _, pair := range t.pairs {
			for _, r := range pair.scalars() {
				write32(&data, uint32(r))
			}
		}
		sort.Slice(t.pairs, func(i, j int) bool {
			a, b := t.pairs[i].scalars(), t.pairs[j].scalars()
			for k := range a {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
			return false
		})
		pairEncode := data.Len()
		for _, pair := range t.pairs {
			for _, r := range pair.scalars() {
				write32(&data, uint32(r))
			}
			write32(&data, pair.code.key)
			data.WriteByte(pair.code.length)
		}
		fmt.Fprintf(&g, "{name:%q,state:%d,shortDecode:%d,decodeIndex:%d,bmpEncode:%d,encodeIndex:%d,encodePages:%d,width:%d,pairDecode:%d,pairEncode:%d,pairCount:%d,ascii:%t,gb18030:%t,shiftControls:%t},\n", name, state, shortIndex, dec, bmpIndex, enc, pageData, t.width, pairDecode, pairEncode, len(t.pairs), t.ascii, gb18030, isProject && project.euc)
		if !isProject && p != *nta && p != *ntaSJIS && !phpFiles[p] {
			fmt.Fprintf(&hashes, "%x  %s\n", sha256.Sum256(raw), filepath.Base(p))
		}
	}
	fmt.Fprintln(&g, "}")
	appendGroups := func(groups [][]byte) (int, int) {
		meta := data.Len()
		offset := uint32(0)
		for _, group := range groups {
			write32(&data, offset)
			write32(&data, uint32(len(group)))
			offset += uint32(len(group))
		}
		payload := data.Len()
		for _, group := range groups {
			data.Write(group)
		}
		return meta, payload
	}
	decMeta, decData := appendGroups(decGroups)
	encMeta, encData := appendGroups(encGroups)
	bmpData := data.Len()
	bmpOffsets := make([]uint32, len(bmpBlocks))
	for i, block := range bmpBlocks {
		bmpOffsets[i] = uint32(data.Len()-bmpData) + 1
		data.Write(block)
	}
	// Replace temporary block IDs with one-based offsets. This preserves
	// deduplication while avoiding a metadata lookup for every BMP scalar.
	for _, index := range bmpIndexes {
		for i := 0; i < 1024; i++ {
			p := index + i*4
			id := binary.LittleEndian.Uint32(data.Bytes()[p : p+4])
			if id != 0 {
				binary.LittleEndian.PutUint32(data.Bytes()[p:p+4], bmpOffsets[id-1])
			}
		}
	}
	shortData := data.Len()
	for _, block := range shortBlocks {
		data.Write(block)
	}
	fmt.Fprintf(&g, "const (\nwideSharedDecodeMeta = %d\nwideSharedDecodeData = %d\nwideSharedEncodeMeta = %d\nwideSharedEncodeData = %d\nwideSharedBMPEncodeData = %d\nwideSharedShortDecode = %d\n)\n", decMeta, decData, encMeta, encData, bmpData, shortData)
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
