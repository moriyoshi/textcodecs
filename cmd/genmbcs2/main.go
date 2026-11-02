// genmbcs2 generates compact tables for stateless ICU MBCS UCMs with at most
// two encoded bytes and one BMP Unicode scalar per mapping.
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

var mappingRE = regexp.MustCompile(`^<U([0-9A-Fa-f]{1,6})>\s+\\x([0-9A-Fa-f]{2})(?:\\x([0-9A-Fa-f]{2}))?\s+\|([0-4])$`)
var stateRE = regexp.MustCompile(`^([0-9A-Fa-f]{1,2})(?:-([0-9A-Fa-f]{1,2}))?(?::([0-9A-Fa-f]+))?(?:\.([iup.]))?$`)
var subcharRE = regexp.MustCompile(`^\\x([0-9A-Fa-f]{2})(?:\\x([0-9A-Fa-f]{2}))?$`)

type table struct {
	base     string
	stateful bool
	states   [][256]byte
	single   [256]uint16
	double   [65536]uint16
	encode   [65536]uint16
	ascii    bool
	subchar  uint16
	subLen   byte
}

func hex(s string) int {
	v, e := strconv.ParseUint(s, 16, 16)
	if e != nil {
		panic(e)
	}
	return int(v)
}
func stateRow(text string) ([256]byte, error) {
	var row [256]byte
	for _, v := range strings.Split(text, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		m := stateRE.FindStringSubmatch(v)
		if m == nil {
			return row, fmt.Errorf("unsupported state entry %q", v)
		}
		lo, hi := hex(m[1]), hex(m[1])
		if m[2] != "" {
			hi = hex(m[2])
		}
		if hi < lo || hi > 255 {
			return row, fmt.Errorf("invalid range %q", v)
		}
		var value byte = 1
		if m[3] != "" && m[4] == "" {
			n := hex(m[3])
			if n > 253 {
				return row, fmt.Errorf("invalid state %q", v)
			}
			value = byte(n + 2)
		}
		if m[4] == "i" {
			value = 0
		}
		if m[3] != "" && m[4] != "" {
			return row, fmt.Errorf("state change on final byte is unsupported: %q", v)
		}
		if m[4] != "" && m[4] != "i" && m[4] != "u" && m[4] != "p" && m[4] != "." {
			return row, fmt.Errorf("unsupported action %q", v)
		}
		for i := lo; i <= hi; i++ {
			row[i] = value
		}
	}
	return row, nil
}
func parse(data []byte) (table, error) {
	var t table
	in, done, max := false, false, false
	class := ""
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
				if class == "DBCS" && len(t.states) == 0 {
					for _, spec := range []string{"0-3f:3, 40:2, 41-fe:1, ff:3", "41-fe", "40", ""} {
						row, err := stateRow(spec)
						if err != nil {
							return t, err
						}
						t.states = append(t.states, row)
					}
				}
				if class == "EBCDIC_STATEFUL" && len(t.states) == 0 {
					for _, spec := range []string{"0-ff, e.i, f.i", "0-3f:4, 40:3, 41-fe:2, ff:4", "41-fe", "40", ""} {
						row, err := stateRow(spec)
						if err != nil {
							return t, err
						}
						t.states = append(t.states, row)
					}
				}
				if class == "" || !max || len(t.states) == 0 {
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
				row, err := stateRow(strings.TrimSpace(strings.TrimPrefix(text, "<icu:state>")))
				if err != nil {
					return t, fmt.Errorf("line %d: %w", line, err)
				}
				t.states = append(t.states, row)
			case "<uconv_class>":
				if len(fields) != 2 || (fields[1] != `"MBCS"` && fields[1] != `"DBCS"` && fields[1] != `"EBCDIC_STATEFUL"`) || class != "" {
					return t, fmt.Errorf("line %d: wrong class", line)
				}
				class = strings.Trim(fields[1], `"`)
				t.stateful = class == "EBCDIC_STATEFUL"
			case "<mb_cur_max>":
				if len(fields) != 2 || fields[1] != "2" || max {
					return t, fmt.Errorf("line %d: wrong width", line)
				}
				max = true
			case "<icu:base>":
				if len(fields) != 2 || t.base != "" {
					return t, fmt.Errorf("line %d: invalid base", line)
				}
				var err error
				t.base, err = strconv.Unquote(fields[1])
				if err != nil || t.base == "" {
					return t, fmt.Errorf("line %d: invalid base", line)
				}
			case "<subchar>":
				if len(fields) != 2 || t.subLen != 0 {
					return t, fmt.Errorf("line %d: invalid substitution", line)
				}
				m := subcharRE.FindStringSubmatch(fields[1])
				if m == nil {
					return t, fmt.Errorf("line %d: unsupported substitution", line)
				}
				t.subchar, t.subLen = uint16(hex(m[1])), 1
				if m[2] != "" {
					t.subchar = t.subchar<<8 | uint16(hex(m[2]))
					t.subLen = 2
				}
			case "<code_set_name>", "<char_name_mask>", "<mb_cur_min>", "<subchar1>", "<icu:charsetFamily>", "<icu:alias>":
			default:
				return t, fmt.Errorf("line %d: unsupported header %s", line, fields[0])
			}
			continue
		}
		if text == "END CHARMAP" {
			done = true
			continue
		}
		m := mappingRE.FindStringSubmatch(text)
		if m == nil {
			return t, fmt.Errorf("line %d: unsupported mapping %q", line, text)
		}
		r := rune(hex(m[1]))
		a := hex(m[2])
		b := 0
		two := m[3] != ""
		if two {
			if a == 0 {
				return t, fmt.Errorf("line %d: two-byte mapping with zero lead", line)
			}
			b = hex(m[3])
		}
		if !utf8.ValidRune(r) || r >= 0xffff {
			return t, fmt.Errorf("line %d: unsupported scalar", line)
		}
		first := t.states[0][a]
		if t.stateful && two {
			first = t.states[1][a]
		}
		if two {
			if first < 2 || int(first-2) >= len(t.states) || t.states[first-2][b] != 1 {
				return t, fmt.Errorf("line %d: byte pair violates state table", line)
			}
		} else if first != 1 {
			return t, fmt.Errorf("line %d: byte violates state table", line)
		}
		val := a
		if two {
			val = a<<8 | b
		}
		if val == 65535 {
			return t, fmt.Errorf("line %d: encoded value overflow", line)
		}
		if m[4] == "0" || m[4] == "3" {
			if two {
				if t.double[val] != 0 {
					return t, fmt.Errorf("line %d: duplicate decode", line)
				}
				t.double[val] = uint16(r) + 1
			} else {
				if t.single[val] != 0 {
					return t, fmt.Errorf("line %d: duplicate decode", line)
				}
				t.single[val] = uint16(r) + 1
			}
		}
		if m[4] == "0" || m[4] == "4" || (m[4] == "1" && r >= 0xe000 && r <= 0xf8ff) {
			if t.encode[r] != 0 {
				return t, fmt.Errorf("line %d: duplicate encode", line)
			}
			t.encode[r] = uint16(val) + 1
		}
	}
	if err := s.Err(); err != nil {
		return t, err
	}
	if !done {
		return t, fmt.Errorf("missing END CHARMAP")
	}
	for i, row := range t.states {
		for _, v := range row {
			if v >= 2 && int(v-2) >= len(t.states) {
				return t, fmt.Errorf("state %d: nonexistent transition", i)
			}
		}
	}
	for _, transition := range t.states[0] {
		if transition < 2 {
			continue
		}
		for _, next := range t.states[transition-2] {
			if next >= 2 {
				return t, fmt.Errorf("state table contains an unsupported third byte")
			}
		}
	}
	t.ascii = true
	// copyASCII handles printable bytes only. Controls can differ (CP932
	// permutes three of them) without invalidating the printable fast path.
	for b := 0x20; b < 0x7f; b++ {
		if t.states[0][b] != 1 || t.single[b] != uint16(b+1) || t.encode[b] != uint16(b+1) {
			t.ascii = false
			break
		}
	}
	return t, nil
}

// These inherited UCMs contain complete mappings despite their <icu:base>
// declarations. Each pinned source has been exhaustively checked against ICU
// with the declared base. Other deltas may contain only changes and must not
// be flattened by this generator without implementing base merging.
var vettedDeltas = map[string]string{
	"ibm-1363_P110-1997": "f1fb66d0aea6e135dcf0736841c8b0f27fec3b5509d3992ea45285d06c2ae0fe",
	"ibm-942_P12A-1999":  "cfb8a3514a7343921cdb08557a65ac9647d568879c31d5836d47c817f5e6e9a4",
	"ibm-943_P130-1999":  "f97e842bafe727f76b1e35b18486254d72a9f9cbdd590487bbbbcde72105e1fe",
	"ibm-949_P110-1999":  "d20199b5b490a8c90b54dfc7e424e4e3df89e80da659c1964c8b277448489798",
	"ibm-950_P110-1999":  "bb774934e0982b82bf71db99f8087cf936f41b91e820c4af35bc4c93924678e9",
	"ibm-971_P100-1995":  "dd01caf1570301366455d137e20cb41309d0f11875ad5f97f1ad6bd5d3942d5b",
	"ibm-937_P110-1999":  "5547aa76e84ad8a3e365e2b9dc2678e5c19f97f6f2e7009c649d2f3930872b87",
	"ibm-939_P120-1999":  "5e449e6abdda67c8ea8d55b348724a5004a835df775bf585428cbdc4edff43af",
	"windows-936-2000":   "40348ea8fdeb62dec8abfdfb8384a75e4ee8c6c641fd85f23f6eaa8cdb4fdad0",
	"windows-949-2000":   "954a3fe3c8e3d0cbd7559dd8c7c3c4ca1fa7ffdfd7374f7259fa0f237908cb16",
	"windows-950-2000":   "6611b22712f101597c790b6bcd6aa1b9e3220e0e3707a7a0485d236d89e03e7e",
}

func put16(b *bytes.Buffer, v uint16) {
	if err := binary.Write(b, binary.LittleEndian, v); err != nil {
		log.Fatal(err)
	}
}
func put32(b *bytes.Buffer, v uint32) {
	if err := binary.Write(b, binary.LittleEndian, v); err != nil {
		log.Fatal(err)
	}
}
func put64(b *bytes.Buffer, v uint64) {
	if err := binary.Write(b, binary.LittleEndian, v); err != nil {
		log.Fatal(err)
	}
}
func main() {
	dir := flag.String("input", "data/icu/mbcs2", "UCM directory")
	statefulDir := flag.String("stateful", "data/icu/ebcdic", "stateful UCM directory")
	out := flag.String("output", "data/icu/mbcs2.dat", "binary output")
	meta := flag.String("metadata", "mbcs2_generated.go", "Go registry")
	manifest := flag.String("manifest", "data/icu/mbcs2.sha256", "source hashes")
	flag.Parse()
	files, err := filepath.Glob(filepath.Join(*dir, "*.ucm"))
	if err != nil || len(files) == 0 {
		log.Fatal("no UCM files")
	}
	statefulFiles, err := filepath.Glob(filepath.Join(*statefulDir, "*.ucm"))
	if err != nil {
		log.Fatal(err)
	}
	files = append(files, statefulFiles...)
	sort.Slice(files, func(i, j int) bool { return filepath.Base(files[i]) < filepath.Base(files[j]) })
	var data, g, hashes bytes.Buffer
	decodeIDs := make(map[string]uint16)
	encodeIDs := make(map[string]uint16)
	var decodeBlocks [][]uint16
	var encodeMasks []uint64
	var encodeOffsets []uint32
	var encodeValues []uint16
	fmt.Fprintln(&g, "// Code generated by cmd/genmbcs2; DO NOT EDIT.\npackage textcodecs\n\nvar mbcs2Entries = [...]mbcs2Entry{")
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".ucm")
		parseInput := raw
		if name == "icu-internal-25546" {
			if fmt.Sprintf("%x", sha256.Sum256(raw)) != "41b0274af9ed0c49013d4eed9fe4a19406f852c1700fdadda16250fd0a4153c5" {
				log.Fatalf("%s: unsupported shift-state table", path)
			}
			// Normalize this pinned SO/SI state table into the representation
			// used by the shared two-byte shift-state engine.
			var normalized strings.Builder
			for _, line := range strings.Split(string(raw), "\n") {
				switch {
				case strings.HasPrefix(line, "<uconv_class>"):
					normalized.WriteString("<uconv_class> \"EBCDIC_STATEFUL\"\n")
				case strings.HasPrefix(line, "<icu:state>"):
					// Add the equivalent normalized rows immediately before CHARMAP.
				case line == "CHARMAP":
					normalized.WriteString("<icu:state> 0-7f, e.i, f.i\n<icu:state> 0-20:3, 21-7e:2, 7f-ff:3\n<icu:state> 21-7e\n<icu:state>\nCHARMAP\n")
				default:
					normalized.WriteString(line + "\n")
				}
			}
			parseInput = []byte(normalized.String())
		}
		t, err := parse(parseInput)
		if err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		if t.subLen == 0 {
			log.Fatalf("%s: missing substitution bytes", path)
		}
		if t.base != "" && vettedDeltas[name] != fmt.Sprintf("%x", sha256.Sum256(raw)) {
			log.Fatalf("%s: inherited mapping not verified for direct generation", path)
		}
		state := data.Len()
		for _, row := range t.states {
			data.Write(row[:])
		}
		single := data.Len()
		for _, v := range t.single {
			put16(&data, v)
		}
		// Similar code pages share identical fixed-size decode and encode
		// blocks. IDs are one-based so an all-zero block needs no storage.
		decodeIndex := data.Len()
		for block := 0; block < 2048; block++ {
			chunk := t.double[block*32 : (block+1)*32]
			var raw bytes.Buffer
			nonzero := false
			for _, v := range chunk {
				put16(&raw, v)
				nonzero = nonzero || v != 0
			}
			var id uint16
			if nonzero {
				var ok bool
				id, ok = decodeIDs[raw.String()]
				if !ok {
					if len(decodeBlocks) >= 65535 {
						log.Fatal("decode block overflow")
					}
					id = uint16(len(decodeBlocks) + 1)
					decodeIDs[raw.String()] = id
					decodeBlocks = append(decodeBlocks, append([]uint16(nil), chunk...))
				}
			}
			put16(&data, id)
		}
		encodeIndex := data.Len()
		for block := 0; block < 1024; block++ {
			chunk := t.encode[block*64 : (block+1)*64]
			var raw bytes.Buffer
			nonzero := false
			for _, v := range chunk {
				put16(&raw, v)
				nonzero = nonzero || v != 0
			}
			var id uint16
			if nonzero {
				var ok bool
				id, ok = encodeIDs[raw.String()]
				if !ok {
					if len(encodeMasks) >= 65535 {
						log.Fatal("encode block overflow")
					}
					id = uint16(len(encodeMasks) + 1)
					encodeIDs[raw.String()] = id
					encodeOffsets = append(encodeOffsets, uint32(len(encodeValues)))
					var mask uint64
					for bit, v := range chunk {
						if v != 0 {
							mask |= uint64(1) << uint(bit)
							encodeValues = append(encodeValues, v)
						}
					}
					encodeMasks = append(encodeMasks, mask)
				}
			}
			put16(&data, id)
		}
		fmt.Fprintf(&g, "{name:%q, state:%d, single:%d, decodeIndex:%d, encodeIndex:%d, subchar:%#x, subLen:%d, ascii:%t, stateful:%t},\n", name, state, single, decodeIndex, encodeIndex, t.subchar, t.subLen, t.ascii, t.stateful)
		fmt.Fprintf(&hashes, "%x  %s\n", sha256.Sum256(raw), filepath.Base(path))
	}
	fmt.Fprintln(&g, "}")
	sharedDecode := data.Len()
	for _, block := range decodeBlocks {
		for _, v := range block {
			put16(&data, v)
		}
	}
	sharedMask := data.Len()
	for _, mask := range encodeMasks {
		put64(&data, mask)
	}
	sharedOffset := data.Len()
	for _, offset := range encodeOffsets {
		put32(&data, offset)
	}
	sharedEncode := data.Len()
	for _, v := range encodeValues {
		put16(&data, v)
	}
	fmt.Fprintf(&g, "const (\nmbcsSharedDecode = %d\nmbcsSharedMask = %d\nmbcsSharedOffset = %d\nmbcsSharedEncode = %d\n)\n", sharedDecode, sharedMask, sharedOffset, sharedEncode)
	code, err := format.Source(g.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	for _, v := range []struct {
		path string
		data []byte
	}{{*out, data.Bytes()}, {*meta, code}, {*manifest, hashes.Bytes()}} {
		if err := os.WriteFile(v.path, v.data, 0644); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Fprintf(os.Stderr, "%d converters, %d table bytes\n", len(files), data.Len())
}
