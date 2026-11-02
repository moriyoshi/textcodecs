package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func workbook(t *testing.T, worksheet, relationship string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="JIS縮退マップ" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": relationship,
		"xl/sharedStrings.xml":       `<sst><si><r><t>first</t></r><rPh><t>ignored</t></rPh><r><t>second</t></r></si></sst>`,
		"xl/worksheets/sheet2.xml":   worksheet,
	}
	for name, content := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadSheet(t *testing.T) {
	rel := `<Relationships><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`
	sheet := `<worksheet><sheetData><row r="3"><c r="A3" t="s"><v>0</v></c><c r="C3" t="inlineStr"><is><t>inline</t></is></c></row></sheetData></worksheet>`
	rows, err := readSheet(workbook(t, sheet, rel))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].number != 3 || rows[0].cells[0] != "firstsecond" || rows[0].cells[1] != "" || rows[0].cells[2] != "inline" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if _, err := readSheet(workbook(t, strings.Replace(sheet, "<v>0</v>", "<v>9</v>", 1), rel)); err == nil {
		t.Fatal("accepted invalid shared string")
	}
	if _, err := readSheet(workbook(t, sheet, strings.Replace(rel, `Target="`, `TargetMode="External" Target="`, 1))); err == nil {
		t.Fatal("accepted external relationship")
	}
}

func TestJISByteForms(t *testing.T) {
	tests := []struct {
		p         position
		euc, sjis []byte
		ok        bool
	}{
		{position{1, 1, 1}, []byte{0xa1, 0xa1}, []byte{0x81, 0x40}, true},
		{position{1, 1, 64}, []byte{0xa1, 0xe0}, []byte{0x81, 0x80}, true},
		{position{1, 2, 1}, []byte{0xa2, 0xa1}, []byte{0x81, 0x9f}, true},
		{position{2, 1, 1}, []byte{0x8f, 0xa1, 0xa1}, []byte{0xf0, 0x40}, true},
		{position{2, 8, 1}, []byte{0x8f, 0xa8, 0xa1}, []byte{0xf0, 0x9f}, true},
		{position{2, 94, 94}, []byte{0x8f, 0xfe, 0xfe}, []byte{0xfc, 0xfc}, true},
		{position{2, 2, 1}, []byte{0x8f, 0xa2, 0xa1}, nil, false},
	}
	for _, tc := range tests {
		got, ok := tc.p.sjis()
		if !bytes.Equal(tc.p.euc(), tc.euc) || ok != tc.ok || !bytes.Equal(got, tc.sjis) {
			t.Errorf("%s: euc %x, sjis %x %v", tc.p, tc.p.euc(), got, ok)
		}
	}
}

func TestShiftJISRejectsUnrepresentableJISRow(t *testing.T) {
	_, err := renderUCM([]mapping{{pos: position{2, 2, 1}, primary: []rune{'あ'}}}, "test", true)
	if err == nil {
		t.Fatal("accepted a JIS plane-2 row that Shift_JIS-2004 cannot represent")
	}
}

func TestUnicodeParsing(t *testing.T) {
	for _, text := range []string{"U+D800", "U+110000", "U+ZZ", "0041"} {
		if _, err := parseScalars(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	got, err := parseScalars("U+304B u+309A")
	if err != nil || string(got) != "か゚" {
		t.Fatalf("got %q, %v", string(got), err)
	}
}
