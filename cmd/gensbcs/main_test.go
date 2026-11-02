package main

import "testing"

const header = `<code_set_name> "example"
<uconv_class> "SBCS"
<mb_cur_max> 1
CHARMAP
`

func TestPrecisionAndPrivateUse(t *testing.T) {
	t0, err := parse([]byte(header + `<U0041> \x41 |0
<U0042> \x42 |3
<U0043> \x43 |1
<UE000> \x44 |1
<U0045> \x45 |4
<U0046> \x46 |2
END CHARMAP
`))
	if err != nil {
		t.Fatal(err)
	}
	if t0.decode[0x41] != 'A'+1 || t0.decode[0x42] != 'B'+1 || t0.decode[0x44] != 0 {
		t.Fatal("wrong decode directions")
	}
	got := map[rune]byte{}
	for _, e := range t0.encode {
		got[e.r] = e.b
	}
	if len(got) != 3 || got['A'] != 0x41 || got[0xe000] != 0x44 || got['E'] != 0x45 {
		t.Fatalf("wrong encode directions: %#v", got)
	}
}
func TestRejectUnsupported(t *testing.T) {
	for _, s := range []string{
		header + `<U0041> \x41\x42 |0` + "\nEND CHARMAP\n",
		header + `<U0041> \x41 |0` + "\n" + `<U0042> \x41 |0` + "\nEND CHARMAP\n",
		header + `<UD800> \x41 |0` + "\nEND CHARMAP\n",
		header + `<UFFFF> \x41 |0` + "\nEND CHARMAP\n",
		header,
	} {
		if _, err := parse([]byte(s)); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
