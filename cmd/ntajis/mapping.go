package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Source positions are JIS X 0213 plane, row, and cell numbers, all one-based.
type position struct{ plane, row, cell int }
type mapping struct {
	pos            position
	primary        []rune
	secondary      []rune
	class          string
	reductionPos   []position
	reductionRunes []rune
	note           string
}

var secondaryRE = regexp.MustCompile(`(?i)^類似字形(u\+[0-9a-f]+)は本文字に変換する。`)
var classes = map[string]bool{"非漢字": true, "追加非漢字": true, "JIS1水": true, "JIS2水": true, "JIS3水": true, "JIS4水": true}

func parsePosition(s string) (position, error) {
	fields := strings.Split(s, "-")
	if len(fields) != 3 {
		return position{}, fmt.Errorf("invalid JIS position %q", s)
	}
	var values [3]int
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return position{}, fmt.Errorf("invalid JIS position %q", s)
		}
		values[i] = n
	}
	p := position{values[0], values[1], values[2]}
	if p.plane < 1 || p.plane > 2 || p.row < 1 || p.row > 94 || p.cell < 1 || p.cell > 94 {
		return position{}, fmt.Errorf("invalid JIS position %q", s)
	}
	return p, nil
}
func (p position) String() string { return fmt.Sprintf("%02d-%02d-%02d", p.plane, p.row, p.cell) }
func (p position) number() int    { return (p.plane-1)*94*94 + (p.row-1)*94 + p.cell - 1 }
func (p position) euc() []byte {
	if p.plane == 1 {
		return []byte{byte(0xa0 + p.row), byte(0xa0 + p.cell)}
	}
	return []byte{0x8f, byte(0xa0 + p.row), byte(0xa0 + p.cell)}
}
func parseScalars(s string) ([]rune, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty Unicode sequence")
	}
	out := make([]rune, 0, len(fields))
	for _, part := range fields {
		if len(part) < 3 || !strings.EqualFold(part[:2], "u+") {
			return nil, fmt.Errorf("invalid Unicode value %q", part)
		}
		n, err := strconv.ParseUint(part[2:], 16, 32)
		if err != nil || n > utf8.MaxRune || !utf8.ValidRune(rune(n)) {
			return nil, fmt.Errorf("invalid Unicode value %q", part)
		}
		out = append(out, rune(n))
	}
	return out, nil
}
func parseMappings(rows []sheetRow) ([]mapping, error) {
	if len(rows) < 2 {
		return nil, fmt.Errorf("workbook has fewer than two header rows")
	}
	first := map[int]string{0: "変換元の文字（JISX0213：1-4水）", 4: "コード変換（1対1変換）", 7: "文字列変換（追加非漢字や、1対ｎの文字変換を行う）", 16: "備考"}
	second := []string{"面区点コード", "Unicode", "字形", "JIS区分", "面区点コード", "Unicode", "字形", "面区点コード①", "面区点コード②", "面区点コード③", "面区点コード④", "Unicode①", "Unicode②", "Unicode③", "Unicode④", "字形"}
	for i, want := range first {
		if rows[0].cells[i] != want {
			return nil, fmt.Errorf("unexpected workbook header row 1 column %d: %q", i+1, rows[0].cells[i])
		}
	}
	for i, want := range second {
		if rows[1].cells[i] != want {
			return nil, fmt.Errorf("unexpected workbook header row 2 column %d: %q", i+1, rows[1].cells[i])
		}
	}
	var result []mapping
	last := -1
	for _, row := range rows[2:] {
		c := row.cells
		if c[0] == "" {
			break
		}
		p, err := parsePosition(c[0])
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", row.number, err)
		}
		if p.number() <= last {
			return nil, fmt.Errorf("row %d: duplicate or unordered JIS position", row.number)
		}
		last = p.number()
		rs, err := parseScalars(c[1])
		if err != nil || len(rs) > 2 {
			return nil, fmt.Errorf("row %d: invalid source Unicode %q", row.number, c[1])
		}
		if !classes[c[3]] {
			return nil, fmt.Errorf("row %d: unknown JIS class %q", row.number, c[3])
		}
		m := mapping{pos: p, primary: rs, class: c[3], note: c[16]}
		if c[4] == "" && c[5] != "" {
			return nil, fmt.Errorf("row %d: incomplete one-to-one reduction", row.number)
		}
		if c[4] != "" {
			for _, value := range c[7:15] {
				if value != "" {
					return nil, fmt.Errorf("row %d: both reduction forms present", row.number)
				}
			}
			q, err := parsePosition(c[4])
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", row.number, err)
			}
			rr, err := parseScalars(c[5])
			if err != nil || len(rr) != 1 {
				return nil, fmt.Errorf("row %d: invalid reduction scalar %q", row.number, c[5])
			}
			m.reductionPos = []position{q}
			m.reductionRunes = rr
		} else {
			for i := 0; i < 4; i++ {
				if c[7+i] == "" && c[11+i] == "" {
					continue
				}
				if c[7+i] == "" || c[11+i] == "" || len(m.reductionPos) != i {
					return nil, fmt.Errorf("row %d: incomplete reduction sequence", row.number)
				}
				q, err := parsePosition(c[7+i])
				if err != nil {
					return nil, fmt.Errorf("row %d: %w", row.number, err)
				}
				rr, err := parseScalars(c[11+i])
				if err != nil || len(rr) != 1 {
					return nil, fmt.Errorf("row %d: invalid reduction scalar %q", row.number, c[11+i])
				}
				m.reductionPos = append(m.reductionPos, q)
				m.reductionRunes = append(m.reductionRunes, rr[0])
			}
		}
		if alt := secondaryRE.FindStringSubmatch(m.note); alt != nil {
			m.secondary, err = parseScalars(alt[1])
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", row.number, err)
			}
		}
		result = append(result, m)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("workbook contains no mappings")
	}
	return result, nil
}
