package textcodecs

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestNTAJISMappings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
	}{
		{"nta-jis-x-0213-euc", 11230}, {"nta-jis-x-0213-sjis", 11421},
	} {
		t.Run(tc.name, func(t *testing.T) { testNTAJISMappings(t, tc.name, tc.count) })
	}
}

func testNTAJISMappings(t *testing.T, name string, expected int) {
	t.Helper()
	raw, err := os.ReadFile("data/nta/" + name + ".ucm")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := Open(name)
	if !ok {
		t.Fatal("missing NTA codec")
	}
	lineRE := regexp.MustCompile(`^(<U[0-9A-F]+>(?:<U[0-9A-F]+>)?) ((?:\\x[0-9A-F]{2})+) \|([013]) `)
	runeRE := regexp.MustCompile(`<U([0-9A-F]+)>`)
	s := bufio.NewScanner(bytes.NewReader(raw))
	count, pairs, fallback := 0, 0, 0
	for s.Scan() {
		m := lineRE.FindStringSubmatch(s.Text())
		if m == nil {
			continue
		}
		if m[3] == "1" {
			fallback++
			continue
		}
		code, err := hex.DecodeString(strings.ReplaceAll(m[2], `\x`, ""))
		if err != nil {
			t.Fatal(err)
		}
		var runes []rune
		for _, match := range runeRE.FindAllStringSubmatch(m[1], -1) {
			n, err := strconv.ParseUint(match[1], 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			runes = append(runes, rune(n))
		}
		if len(runes) == 2 {
			pairs++
		}
		text := []byte(string(runes))
		var out [16]byte
		n, used, err := c.Decode(out[:], code, true)
		if err != nil || used != len(code) || !bytes.Equal(out[:n], text) {
			t.Fatalf("decode %x: %x, %d, %v; want %x", code, out[:n], used, err, text)
		}
		if m[3] == "0" {
			n, used, err = c.Encode(out[:], text, true)
			if err != nil || used != len(text) || !bytes.Equal(out[:n], code) {
				t.Fatalf("encode %U: %x, %d, %v; want %x", runes, out[:n], used, err, code)
			}
		}
		count++
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if count != expected || pairs != 25 || fallback != 7 {
		t.Fatalf("got %d mappings, %d pairs, %d fallbacks", count, pairs, fallback)
	}
	if name == "nta-jis-x-0213-euc" {
		var out [8]byte
		if _, _, err := c.Decode(out[:], []byte("A"), true); err != ErrInvalid {
			t.Fatalf("ASCII unexpectedly accepted: %v", err)
		}
		if _, _, err := c.Decode(out[:], []byte{0x8e, 0xb1}, true); err != ErrInvalid {
			t.Fatalf("X 0201 kana unexpectedly accepted: %v", err)
		}
	}
}

func TestNTAJISShiftJISCoverage(t *testing.T) {
	c, ok := Open("nta-jis-x-0213-sjis")
	if !ok {
		t.Fatal("missing codec")
	}
	for _, tc := range []struct {
		code []byte
		text string
	}{
		{[]byte{0x41}, "A"}, {[]byte{0x5c}, "\u00a5"}, {[]byte{0x7e}, "\u203e"},
		{[]byte{0xb1}, "\uff71"}, {[]byte{0x81, 0x5f}, "\uff3c"},
		{[]byte{0xf0, 0x40}, "\U00020089"}, {[]byte{0xf0, 0x9f}, "\u5b96"},
	} {
		var out [16]byte
		n, used, err := c.Decode(out[:], tc.code, true)
		if err != nil || used != len(tc.code) || string(out[:n]) != tc.text {
			t.Fatalf("decode %x: %q, %d, %v", tc.code, out[:n], used, err)
		}
		n, used, err = c.Encode(out[:], []byte(tc.text), true)
		if err != nil || used != len(tc.text) || !bytes.Equal(out[:n], tc.code) {
			t.Fatalf("encode %U: %x, %d, %v", []rune(tc.text), out[:n], used, err)
		}
	}
}
