package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMappings(t *testing.T) {
	entries, err := mappings("ibm-943_P15A-2003", "82", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0xa0]; got.Text != "あ" || len(got.Codepoints) != 1 || got.Codepoints[0] != "U+3042" {
		t.Fatalf("82 A0: %+v", got)
	}
	if _, err := mappings("no-such-codec", "", 0); err == nil {
		t.Fatal("unknown codec accepted")
	}
	if _, err := mappings("ibm-943_P15A-2003", "8", 0); err == nil {
		t.Fatal("partial prefix accepted")
	}
}

func TestHandlers(t *testing.T) {
	h := handler()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/", http.StatusOK}, {"/api/codecs", http.StatusOK}, {"/api/single-byte-codecs", http.StatusOK},
		{"/api/mappings?codec=unknown", http.StatusBadRequest},
		{"/api/mappings?codec=ibm-943_P15A-2003&prefix=82", http.StatusOK},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s: got %d, want %d", tc.path, w.Code, tc.status)
		}
	}
}

func TestShiftNavigation(t *testing.T) {
	initial, err := mappings("ibm-930_P120-1999", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := initial[0x0e].ShiftTo; got != "0E" {
		t.Fatalf("SO shift = %q", got)
	}
	shifted, err := mappings("ibm-930_P120-1999", "0E", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := shifted[0x0f].ShiftTo; got != "initial" {
		t.Fatalf("SI shift = %q", got)
	}
	ordinary, err := mappings("ibm-943_P15A-2003", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary[0x0e].ShiftTo != "" {
		t.Fatal("ordinary control incorrectly marked as a shift")
	}
}

func TestTwoByteViewOmitsAdjacentCharacters(t *testing.T) {
	entries, err := mappings("ibm-943_P15A-2003", "41", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0x42]; got.Status != "multiple characters" || got.Text != "" {
		t.Fatalf("41 42: %+v", got)
	}
	entries, err = mappings("ibm-943_P15A-2003", "82", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0xa0]; got.Text != "あ" {
		t.Fatalf("82 A0: %+v", got)
	}
}

func TestPrefixControls(t *testing.T) {
	entries, err := mappings("ibm-943_P15A-2003", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !entries[0x82].Expandable {
		t.Fatal("CP932 lead byte should be expandable")
	}
	if entries[0x41].Expandable {
		t.Fatal("ASCII character should not be expandable")
	}
	if entries[0xff].Expandable {
		t.Fatal("invalid byte should not be expandable")
	}
}

func TestUnicodeNames(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want string
	}{
		{'あ', "HIRAGANA LETTER A"}, {'漢', "CJK UNIFIED IDEOGRAPH-6F22"},
		{'가', "HANGUL SYLLABLE GA"}, {0, "Name unavailable"},
	} {
		if got := unicodeName(tc.r); got != tc.want {
			t.Errorf("%U: got %q, want %q", tc.r, got, tc.want)
		}
	}
	entries, err := mappings("ibm-943_P15A-2003", "82", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0xa0].Names; len(got) != 1 || got[0] != "HIRAGANA LETTER A" {
		t.Fatalf("names: %v", got)
	}
}

func TestLongSequenceViews(t *testing.T) {
	for _, tc := range []struct {
		codec, prefix string
		width         int
		last          byte
		text          string
	}{
		{"euc-jp-2007", "8FA2", 3, 0xAF, "˘"},
		{"euc-tw-2014", "8EA4A1", 4, 0xA6, "丂"},
	} {
		entries, err := mappings(tc.codec, tc.prefix, tc.width)
		if err != nil {
			t.Fatal(err)
		}
		if got := entries[tc.last]; got.Text != tc.text {
			t.Errorf("%s %s%02X: got %+v", tc.codec, tc.prefix, tc.last, got)
		}
	}
	entries, err := mappings("euc-jp-2007", "4142", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := entries[0x43]; got.Status != "multiple characters" {
		t.Fatalf("three ASCII bytes: %+v", got)
	}
}

func TestMinimapShowsMappedRanges(t *testing.T) {
	cells, err := minimap(context.Background(), "ibm-943_P15A-2003", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range cells {
		if cell.Index == 0x410+2 {
			t.Fatal("adjacent ASCII characters marked as a mapping")
		}
		if cell.Index == 0x820 {
			return
		}
	}
	t.Fatal("missing mapped 82A0–82AF range")
}

func TestMinimapCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := minimap(ctx, "gb18030-2022", "", 4); err != context.Canceled {
		t.Fatalf("got %v, want cancellation", err)
	}
}
