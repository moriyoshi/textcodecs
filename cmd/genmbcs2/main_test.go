package main

import "testing"

func TestStateOverrideAndActions(t *testing.T) {
	row, err := stateRow("0-7f, 40-4f:1, 45:2, 50-60.u, 55.i")
	if err != nil {
		t.Fatal(err)
	}
	if row[0x20] != 1 || row[0x40] != 3 || row[0x45] != 4 || row[0x50] != 1 || row[0x55] != 0 || row[0x80] != 0 {
		t.Fatal(row)
	}
	if _, err := stateRow("0-ff:1.s"); err == nil {
		t.Fatal("accepted stateful shift")
	}
}
func TestMappingPrecision(t *testing.T) {
	ucm := `<code_set_name> "test"
<uconv_class> "MBCS"
<mb_cur_max> 2
<icu:state> 0-7f, 81:1
<icu:state> 40-7e
CHARMAP
<U0041> \x41 |0
<U4E00> \x81\x40 |0
<U4E01> \x81\x41 |3
<UE000> \x81\x42 |1
<U4E03> \x81\x43 |1
<U4E04> \x81\x44 |4
<U4E05> \x81\x45 |2
END CHARMAP
`
	m, err := parse([]byte(ucm))
	if err != nil {
		t.Fatal(err)
	}
	if m.single[0x41] != 0x42 || m.double[0x8140] != 0x4e01 || m.double[0x8141] != 0x4e02 || m.double[0x8142] != 0 {
		t.Fatal("bad decode")
	}
	if m.encode[0x4e00] != 0x8141 || m.encode[0xe000] != 0x8143 || m.encode[0x4e04] != 0x8145 || m.encode[0x4e01] != 0 || m.encode[0x4e03] != 0 || m.encode[0x4e05] != 0 {
		t.Fatal("bad encode")
	}
}

func TestSubstitutionHeader(t *testing.T) {
	for _, tc := range []struct {
		header string
		value  uint16
		length byte
	}{
		{`<subchar> \x3F`, 0x3f, 1},
		{`<subchar> \xFC\xFC`, 0xfcfc, 2},
	} {
		input := `<uconv_class> "MBCS"
<mb_cur_max> 2
<icu:state> 0-ff
` + tc.header + "\nCHARMAP\n<U0041> \\x41 |0\nEND CHARMAP\n"
		m, err := parse([]byte(input))
		if err != nil || m.subchar != tc.value || m.subLen != tc.length {
			t.Fatalf("%s: %#v %v", tc.header, m, err)
		}
	}
}
