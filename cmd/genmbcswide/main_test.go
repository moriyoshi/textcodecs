package main

import "testing"

func TestWideMappingAndSupplementaryPrivateFallback(t *testing.T) {
	input := `<code_set_name> "test"
<uconv_class> "MBCS"
<mb_cur_max> 4
<icu:state> 0-7f, 81:1
<icu:state> 40:2, 41
<icu:state> 42:3
<icu:state> 43
CHARMAP
<U0041> \x41 |0
<U20000> \x81\x40\x42\x43 |0
<UF0000> \x81\x41 |1
<UFFFE> \x42 |0
END CHARMAP
`
	table, err := parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if table.decode[0x81404243] != 0x20000 || table.encode[0x20000].length != 4 {
		t.Fatal("four-byte mapping")
	}
	if table.encode[0xf0000].key != 0x81410000 {
		t.Fatal("private-use fallback")
	}
	if _, ok := table.decode[0x42000000]; ok {
		t.Fatal("reserved ICU sentinel decodes")
	}
	if _, ok := table.encode[0xfffe]; !ok {
		t.Fatal("reserved ICU sentinel should encode")
	}
}
