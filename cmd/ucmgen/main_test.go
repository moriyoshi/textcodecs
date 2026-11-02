package main

import (
	"strings"
	"testing"
)

const header = `<code_set_name> "test"
<uconv_class> "SBCS"
<mb_cur_max> 1
CHARMAP
`

func TestParseDirections(t *testing.T) {
	input := header + `<U0000> \x00 |0
<U0041> \x41 |0
<U20AC> \x80 |0
<U0042> \x42 |3
<U0043> \x43 |1
END CHARMAP
`
	got, err := parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if got.decode[0] != 1 || got.decode[0x80] != 0x20ad || got.decode[0x42] != 'B'+1 || got.decode[0x43] != 0 {
		t.Fatal("incorrect decode mapping", got.decode)
	}
	if got.encode['€'] != 0x80 {
		t.Fatal("missing round trip")
	}
	if _, ok := got.encode['B']; ok {
		t.Fatal("reverse fallback must not encode")
	}
	if _, ok := got.encode['C']; ok {
		t.Fatal("forward fallback must not encode by default")
	}
}
func TestRejectUnsupported(t *testing.T) {
	for _, input := range []string{
		strings.Replace(header, `"SBCS"`, `"MBCS"`, 1) + "END CHARMAP\n",
		header + `<U0041> \x41\x42 |0` + "\nEND CHARMAP\n",
		header + `<U0041> \x41 |2` + "\nEND CHARMAP\n",
		header + `<U0041> \x41 |0` + "\n" + `<U0042> \x41 |0` + "\nEND CHARMAP\n",
		header + `<UD800> \x41 |0` + "\nEND CHARMAP\n",
		header,
	} {
		if _, err := parse([]byte(input)); err == nil {
			t.Errorf("accepted unsupported input %q", input)
		}
	}
}
