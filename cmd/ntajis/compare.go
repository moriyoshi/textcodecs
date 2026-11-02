package main

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"textcodecs"
)

type compareSpec struct {
	name, description string
	shift, plane1Only bool
}
type comparison struct {
	spec                                                     compareSpec
	decodeSame, decodeDifferent, decodeMissing, existingOnly int
	encodeSame, encodeDifferent, encodeMissing               int
	altSame, altDifferent, altMissing                        int
}

var comparisons = []compareSpec{
	{name: "euc-jis-2004", description: "planes 1 and 2"},
	{name: "shift_jis-2004", description: "planes 1 and 2", shift: true},
	{name: "euc-jp-2007", description: "plane 1 only", plane1Only: true},
	{name: "ibm-943_P15A-2003", description: "plane 1 only", shift: true, plane1Only: true},
}

// sjis converts JIS X 0213 plane/row/cell into Shift_JIS-2004 bytes. Plane 2
// allocates 26 rows in a nonnumeric order; all other rows have no byte form.
func (p position) sjis() ([]byte, bool) {
	index := p.row - 1
	if p.plane == 2 {
		rows := [...]int{1, 8, 3, 4, 5, 12, 13, 14, 15, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93, 94}
		index = -1
		for i, row := range rows {
			if row == p.row {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, false
		}
	}
	lead := 0x81 + index/2
	if p.plane == 2 {
		lead = 0xf0 + index/2
	} else if lead > 0x9f {
		lead += 0x40
	}
	trail := 0x40 + p.cell - 1
	if index%2 == 1 {
		trail = 0x9f + p.cell - 1
	} else if trail >= 0x7f {
		trail++
	}
	return []byte{byte(lead), byte(trail)}, true
}
func (s compareSpec) code(p position) ([]byte, bool) {
	if s.plane1Only && p.plane != 1 {
		return nil, false
	}
	if s.shift {
		return p.sjis()
	}
	return p.euc(), true
}
func bytesText(b []byte) string {
	if len(b) == 0 {
		return "<empty>"
	}
	return fmt.Sprintf("% X", b)
}
func decode(codec textcodecs.Codec, code []byte) (string, error) {
	codec.Reset()
	var dst [32]byte
	n, used, err := codec.Decode(dst[:], code, true)
	if err != nil {
		return "", err
	}
	if used != len(code) {
		return "", fmt.Errorf("consumed %d of %d bytes", used, len(code))
	}
	return string(dst[:n]), nil
}
func encode(codec textcodecs.Codec, text string) ([]byte, error) {
	codec.Reset()
	var dst [32]byte
	n, used, err := codec.Encode(dst[:], []byte(text), true)
	if err != nil {
		return nil, err
	}
	if used != len(text) {
		return nil, fmt.Errorf("consumed %d of %d UTF-8 bytes", used, len(text))
	}
	return append([]byte(nil), dst[:n]...), nil
}
func comparisonFiles(mappings []mapping, hash string) ([]byte, []byte, error) {
	byPos := make(map[position]mapping, len(mappings))
	for _, m := range mappings {
		byPos[m.pos] = m
	}
	var detail bytes.Buffer
	writer := csv.NewWriter(&detail)
	writer.Comma = '\t'
	writer.UseCRLF = false
	if err := writer.Write([]string{"codec", "direction", "jis_position", "encoded_bytes", "status", "nta_unicode", "existing"}); err != nil {
		return nil, nil, err
	}
	var stats []comparison
	record := func(fields ...string) error { return writer.Write(fields) }
	for _, spec := range comparisons {
		for _, m := range mappings {
			if spec.plane1Only && m.pos.plane != 1 {
				continue
			}
			if _, ok := spec.code(m.pos); !ok {
				return nil, nil, fmt.Errorf("%s has no byte representation for %s", spec.name, m.pos)
			}
		}
		codec, ok := textcodecs.Open(spec.name)
		if !ok {
			return nil, nil, fmt.Errorf("missing existing codec %s", spec.name)
		}
		c := comparison{spec: spec}
		for plane := 1; plane <= 2; plane++ {
			for row := 1; row <= 94; row++ {
				for cell := 1; cell <= 94; cell++ {
					p := position{plane, row, cell}
					code, ok := spec.code(p)
					if !ok {
						continue
					}
					m, present := byPos[p]
					got, err := decode(codec, code)
					if !present {
						if err == nil {
							c.existingOnly++
							if err := record(spec.name, "decode", p.String(), bytesText(code), "existing-only", "<none>", scalarText([]rune(got))); err != nil {
								return nil, nil, err
							}
						}
						continue
					}
					want := string(m.primary)
					switch {
					case err != nil:
						c.decodeMissing++
						if err := record(spec.name, "decode", p.String(), bytesText(code), "missing", scalarText(m.primary), err.Error()); err != nil {
							return nil, nil, err
						}
					case got != want:
						c.decodeDifferent++
						if err := record(spec.name, "decode", p.String(), bytesText(code), "different", scalarText(m.primary), scalarText([]rune(got))); err != nil {
							return nil, nil, err
						}
					default:
						c.decodeSame++
					}
					actual, err := encode(codec, want)
					switch {
					case err != nil:
						c.encodeMissing++
						if err := record(spec.name, "encode", p.String(), bytesText(code), "missing", scalarText(m.primary), err.Error()); err != nil {
							return nil, nil, err
						}
					case !bytes.Equal(actual, code):
						c.encodeDifferent++
						if err := record(spec.name, "encode", p.String(), bytesText(code), "different", scalarText(m.primary), bytesText(actual)); err != nil {
							return nil, nil, err
						}
					default:
						c.encodeSame++
					}
					if len(m.secondary) > 0 {
						actual, err := encode(codec, string(m.secondary))
						switch {
						case err != nil:
							c.altMissing++
							if err := record(spec.name, "encode-alternate", p.String(), bytesText(code), "missing", scalarText(m.secondary), err.Error()); err != nil {
								return nil, nil, err
							}
						case !bytes.Equal(actual, code):
							c.altDifferent++
							if err := record(spec.name, "encode-alternate", p.String(), bytesText(code), "different", scalarText(m.secondary), bytesText(actual)); err != nil {
								return nil, nil, err
							}
						default:
							c.altSame++
						}
					}
				}
			}
		}
		stats = append(stats, c)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, nil, err
	}
	classes := map[string]int{}
	pairs, alternates, reductions := 0, 0, 0
	for _, m := range mappings {
		classes[m.class]++
		if len(m.primary) > 1 {
			pairs++
		}
		if len(m.secondary) > 0 {
			alternates++
		}
		if len(m.reductionPos) > 0 {
			reductions++
		}
	}
	var report bytes.Buffer
	fmt.Fprintf(&report, "# National Tax Agency JIS mapping comparison\n\n")
	fmt.Fprintf(&report, "Source: [National Tax Agency JIS Reduction Map, version 1.0.0](https://www.houjin-bangou.nta.go.jp/download/); spreadsheet SHA-256 `%s`. This is an independently generated analysis of a National Tax Agency work.\n\n", hash)
	fmt.Fprintf(&report, "The spreadsheet contains **%d JIS positions**, including %d two-scalar Unicode mappings, %d alternate Unicode mappings, and %d rows with reduction information. Reduction values describe transliteration and are retained as UCM comments; they are not applied by these codec comparisons.\n\n", len(mappings), pairs, alternates, reductions)
	fmt.Fprintf(&report, "The generated UCM uses EUC-JIS-2004 byte forms for the spreadsheet's JIS positions only; it does not include ASCII or JIS X 0201 kana. Decoding compares each JIS position; encoding compares each primary Unicode sequence against that position. ‘Missing’ includes invalid or unmapped inputs. ‘Existing only’ counts positions mapped by the existing codec but absent from the spreadsheet. Differences are listed in [nta-jis-comparison.tsv](nta-jis-comparison.tsv).\n\n")
	fmt.Fprintln(&report, "| Existing codec | Scope | Decode same | Decode different | NTA-only | Existing only | Encode same | Encode different | Encode missing |")
	fmt.Fprintln(&report, "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, s := range stats {
		fmt.Fprintf(&report, "| `%s` | %s | %d | %d | %d | %d | %d | %d | %d |\n", s.spec.name, s.spec.description, s.decodeSame, s.decodeDifferent, s.decodeMissing, s.existingOnly, s.encodeSame, s.encodeDifferent, s.encodeMissing)
	}
	fmt.Fprintln(&report, "\nAlternate Unicode values, compared with default encoder behavior:")
	fmt.Fprintln(&report, "\n| Existing codec | Same | Different | Missing |\n| --- | ---: | ---: | ---: |")
	for _, s := range stats {
		fmt.Fprintf(&report, "| `%s` | %d | %d | %d |\n", s.spec.name, s.altSame, s.altDifferent, s.altMissing)
	}
	fmt.Fprintln(&report, "\nThe `encode-alternate` rows in the TSV describe differences for those values, which are fallback mappings in the UCM when unambiguous. The ICU EUC-JP and CP932 comparisons are limited to plane 1 because their extension areas do not represent JIS X 0213 plane 2. `shift_jis-2004` uses the allocated plane-2 rows of Shift_JIS-2004. Comparisons use the checked-in codec tables and their strict, default fallback behavior.")
	return report.Bytes(), detail.Bytes(), nil
}
