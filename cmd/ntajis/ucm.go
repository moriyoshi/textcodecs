package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type ucmEntry struct {
	scalars   []rune
	code      []byte
	precision int
	comment   string
}

func scalarText(rs []rune) string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = fmt.Sprintf("U+%04X", r)
	}
	return strings.Join(out, " ")
}
func ucmScalars(rs []rune) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "<U%04X>", r)
	}
	return b.String()
}
func ucmBytes(code []byte) string {
	var b strings.Builder
	for _, v := range code {
		fmt.Fprintf(&b, `\x%02X`, v)
	}
	return b.String()
}
func mappingComment(m mapping) string {
	var b strings.Builder
	fmt.Fprintf(&b, "NTA JIS %s; class=%s", m.pos, m.class)
	if len(m.secondary) > 0 {
		fmt.Fprintf(&b, "; alternate=%s", scalarText(m.secondary))
	}
	if len(m.reductionPos) > 0 {
		pos := make([]string, len(m.reductionPos))
		for i, p := range m.reductionPos {
			pos[i] = p.String()
		}
		fmt.Fprintf(&b, "; reduction=%s (%s)", strings.Join(pos, "+"), scalarText(m.reductionRunes))
	}
	if m.note != "" {
		fmt.Fprintf(&b, "; note=%s", strings.ReplaceAll(strings.ReplaceAll(m.note, "\n", " "), "\r", " "))
	}
	return b.String()
}
func renderUCM(mappings []mapping, workbookHash string, shift bool) ([]byte, error) {
	var entries []ucmEntry
	seenPrimary := map[string]bool{}
	if shift {
		// Shift_JIS-2004 uses JIS X 0201 Roman and halfwidth kana in addition
		// to the two JIS X 0213 planes supplied by the NTA spreadsheet.
		for b := 0; b < 0x80; b++ {
			r := rune(b)
			if b == 0x5c {
				r = 0x00a5
			} else if b == 0x7e {
				r = 0x203e
			}
			entries = append(entries, ucmEntry{[]rune{r}, []byte{byte(b)}, 0, "JIS X 0201 Roman"})
			seenPrimary[string(r)] = true
		}
		for b := 0xa1; b <= 0xdf; b++ {
			r := rune(0xfec0 + b)
			entries = append(entries, ucmEntry{[]rune{r}, []byte{byte(b)}, 0, "JIS X 0201 kana"})
			seenPrimary[string(r)] = true
		}
	}
	codeFor := func(p position) ([]byte, error) {
		if shift {
			code, ok := p.sjis()
			if !ok {
				return nil, fmt.Errorf("JIS position %s has no Shift_JIS-2004 representation", p)
			}
			return code, nil
		}
		return p.euc(), nil
	}
	for _, m := range mappings {
		code, err := codeFor(m.pos)
		if err != nil {
			return nil, err
		}
		key := string(m.primary)
		precision := 0
		if seenPrimary[key] {
			precision = 3
		} else {
			seenPrimary[key] = true
		}
		entries = append(entries, ucmEntry{m.primary, code, precision, mappingComment(m)})
	}
	seenAlt := map[string]bool{}
	for _, m := range mappings {
		if len(m.secondary) == 0 {
			continue
		}
		key := string(m.secondary)
		// A preferred round-trip or earlier fallback takes precedence. The original
		// alternate remains in the comment even when it cannot be a unique mapping.
		if seenPrimary[key] || seenAlt[key] {
			continue
		}
		seenAlt[key] = true
		code, err := codeFor(m.pos)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ucmEntry{m.secondary, code, 1, "NTA alternate for JIS " + m.pos.String()})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if string(a.scalars) != string(b.scalars) {
			return string(a.scalars) < string(b.scalars)
		}
		if a.precision != b.precision {
			return a.precision < b.precision
		}
		return bytes.Compare(a.code, b.code) < 0
	})
	var out bytes.Buffer
	fmt.Fprintln(&out, `# Derived from the National Tax Agency (Japan) JIS Reduction Map, version 1.0.0.
# Source: https://www.houjin-bangou.nta.go.jp/download/ (JIS縮退マップ)`)
	fmt.Fprintf(&out, "# Spreadsheet SHA-256: %s\n", workbookHash)
	fmt.Fprintln(&out, `# The spreadsheet is a work of the National Tax Agency. This file is an
# independently generated adaptation; it is not an official NTA publication.`)
	if shift {
		fmt.Fprintln(&out, `# The JIS X 0213 planes use Shift_JIS-2004 byte forms. JIS X 0201 Roman
# and kana mappings are included separately from the NTA spreadsheet.
# NTA reduction sequences are comments; conversion does not transliterate.
<code_set_name> "nta-jis-x-0213-sjis"
<char_name_mask> "AXXXX"
<mb_cur_max> 2
<mb_cur_min> 1
<uconv_class> "MBCS"
<subchar> \x3F
<icu:state> 00-7f, 81-9f:1, a1-df, e0-fc:1
<icu:state> 40-7e.p, 80-fc.p
CHARMAP`)
	} else {
		fmt.Fprintln(&out, `# Byte sequences are the JIS X 0213 planes in EUC-JIS-2004 form. This table
# intentionally excludes ASCII and JIS X 0201 kana. NTA reduction sequences
# are recorded in comments; UCM conversion does not perform transliteration.
<code_set_name> "nta-jis-x-0213-euc"
<char_name_mask> "AXXXX"
<mb_cur_max> 3
<mb_cur_min> 2
<uconv_class> "MBCS"
<subchar> \xA1\xA1
<icu:state> 8f:2, a1-fe:1
<icu:state> a1-fe.p
<icu:state> a1-fe:1
CHARMAP`)
	}
	for _, e := range entries {
		fmt.Fprintf(&out, "%s %s |%d # %s\n", ucmScalars(e.scalars), ucmBytes(e.code), e.precision, e.comment)
	}
	fmt.Fprintln(&out, "END CHARMAP")
	return out.Bytes(), nil
}
