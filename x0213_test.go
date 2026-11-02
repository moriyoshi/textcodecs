package textcodecs

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestX0213PublishedMappings(t *testing.T) {
	for _, tc := range []struct {
		name, source, hash string
		count              int
	}{
		{"euc-jis-2004", "data/x0213/euc-jis-2004-std.txt", "efcf5545f25abc359796ecebededdfb13b44d3cfe4f9c82aa42e3b9aae80c281", 11456},
		{"shift_jis-2004", "data/x0213/sjis-0213-2004-std.txt", "0def02d3e7a05b126318338a4b71cf30d281816ef6b340c74bd3e906df1ee21a", 11424},
	} {
		t.Run(tc.name, func(t *testing.T) { testX0213PublishedMappings(t, tc.name, tc.source, tc.hash, tc.count) })
	}
}

func testX0213PublishedMappings(t *testing.T, name, source, hash string, expected int) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != hash {
		t.Fatalf("unexpected source hash %s", got)
	}
	codec, ok := Open(name)
	if !ok {
		t.Fatalf("missing %s", name)
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	count, pairs := 0, 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "0x") || !strings.HasPrefix(fields[1], "U+") {
			continue
		}
		encoded, err := hex.DecodeString(fields[0][2:])
		if err != nil {
			t.Fatal(err)
		}
		var runes []rune
		for _, part := range strings.Split(fields[1][2:], "+") {
			n, err := strconv.ParseUint(part, 16, 32)
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
		n, used, err := codec.Decode(out[:], encoded, true)
		if err != nil || used != len(encoded) || !bytes.Equal(out[:n], text) {
			t.Fatalf("decode %x: got %x, consumed %d, err %v; want %x", encoded, out[:n], used, err, text)
		}
		n, used, err = codec.Encode(out[:], text, true)
		if err != nil || used != len(text) || !bytes.Equal(out[:n], encoded) {
			t.Fatalf("encode %U: got %x, consumed %d, err %v; want %x", runes, out[:n], used, err, encoded)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != expected || pairs != 25 {
		t.Fatalf("read %d mappings including %d pairs", count, pairs)
	}
}

func TestX0213StreamingPairsAndShifts(t *testing.T) {
	c, ok := Open("euc-jis-2004")
	if !ok {
		t.Fatal("missing codec")
	}
	out := make([]byte, 16)
	for _, tc := range []struct {
		in   []byte
		eof  bool
		want error
	}{
		{[]byte{0x8e}, false, ErrShortSrc}, {[]byte{0x8f, 0xa1}, false, ErrShortSrc},
	} {
		n, used, err := c.Decode(out, tc.in, tc.eof)
		if n != 0 || used != 0 || err != tc.want {
			t.Fatalf("decode %x: %d %d %v", tc.in, n, used, err)
		}
	}
	n, used, err := c.Decode(out, []byte{0x8e, 0xb1}, true)
	if err != nil || used != 2 || string(out[:n]) != "ｱ" {
		t.Fatalf("SS2: %q %d %v", out[:n], used, err)
	}
	n, used, err = c.Decode(out[:5], []byte{0xa4, 0xf7}, true)
	if err != ErrShortDst || n != 0 || used != 0 {
		t.Fatalf("short pair destination: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(out, []byte{0xa4, 0xf7}, true)
	if err != nil || used != 2 || string(out[:n]) != "か\u309a" {
		t.Fatalf("pair decode: %q %d %v", out[:n], used, err)
	}
	n, used, err = c.Encode(out, []byte("か"), false)
	if err != ErrShortSrc || n != 0 || used != 0 {
		t.Fatalf("pair prefix: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(out[:1], []byte("か\u309a"), true)
	if err != ErrShortDst || n != 0 || used != 0 {
		t.Fatalf("short pair encode: %d %d %v", n, used, err)
	}
	n, used, err = c.Encode(out, []byte("か\u309a"), true)
	if err != nil || used != len("か\u309a") || !bytes.Equal(out[:n], []byte{0xa4, 0xf7}) {
		t.Fatalf("pair encode: %x %d %v", out[:n], used, err)
	}
	// Standalone SS2 is a control at EOF, but takes precedence as a shift
	// when followed by a valid katakana byte.
	n, used, err = c.Decode(out, []byte{0x8e}, true)
	if err != nil || used != 1 || string(out[:n]) != "\u008e" {
		t.Fatalf("standalone SS2: %x %d %v", out[:n], used, err)
	}
}

func TestX0213StreamingSplits(t *testing.T) {
	c, ok := Open("euc-jis-2004")
	if !ok {
		t.Fatal("missing codec")
	}
	encoded := []byte{'A', 0x8e, 0xb1, 0xa4, 0xf7, 0x8f, 0xa1, 0xa1}
	text := []byte("Aｱか\u309a\U00020089")
	for split := 0; split <= len(encoded); split++ {
		out := make([]byte, 64)
		n1, used1, err := c.Decode(out, encoded[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("decode split %d: %v", split, err)
		}
		n2, used2, err := c.Decode(out[n1:], encoded[used1:], true)
		if err != nil || used1+used2 != len(encoded) || !bytes.Equal(out[:n1+n2], text) {
			t.Fatalf("decode split %d: %x, consumed %d+%d, %v", split, out[:n1+n2], used1, used2, err)
		}
	}
	for split := 0; split <= len(text); split++ {
		out := make([]byte, 64)
		n1, used1, err := c.Encode(out, text[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("encode split %d: %v", split, err)
		}
		n2, used2, err := c.Encode(out[n1:], text[used1:], true)
		if err != nil || used1+used2 != len(text) || !bytes.Equal(out[:n1+n2], encoded) {
			t.Fatalf("encode split %d: %x, consumed %d+%d, %v", split, out[:n1+n2], used1, used2, err)
		}
	}
}

func TestX0213ShiftJISStreaming(t *testing.T) {
	c, ok := Open("shift_jis-2004")
	if !ok {
		t.Fatal("missing Shift_JIS-2004")
	}
	encoded := []byte{'A', 0x5c, 0x7e, 0x81, 0x5f, 0x81, 0xb0, 0x82, 0xf5, 0xf0, 0x40, 0xb1}
	text := []byte("A\u00a5\u203e\\~か\u309a\U00020089ｱ")
	for split := 0; split <= len(encoded); split++ {
		out := make([]byte, 64)
		n1, used1, err := c.Decode(out, encoded[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("decode split %d: %v", split, err)
		}
		n2, used2, err := c.Decode(out[n1:], encoded[used1:], true)
		if err != nil || used1+used2 != len(encoded) || !bytes.Equal(out[:n1+n2], text) {
			t.Fatalf("decode split %d: %x, consumed %d+%d, %v", split, out[:n1+n2], used1, used2, err)
		}
	}
	for split := 0; split <= len(text); split++ {
		out := make([]byte, 64)
		n1, used1, err := c.Encode(out, text[:split], false)
		if err != nil && err != ErrShortSrc {
			t.Fatalf("encode split %d: %v", split, err)
		}
		n2, used2, err := c.Encode(out[n1:], text[used1:], true)
		if err != nil || used1+used2 != len(text) || !bytes.Equal(out[:n1+n2], encoded) {
			t.Fatalf("encode split %d: %x, consumed %d+%d, %v", split, out[:n1+n2], used1, used2, err)
		}
	}
	var out [16]byte
	n, used, err := c.Decode(out[:], []byte{0x82}, true)
	if err != ErrInvalid || n != 0 || used != 0 {
		t.Fatalf("truncated lead: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(out[:], []byte{0x82, 0x7f}, true)
	if err != ErrInvalid || n != 0 || used != 0 {
		t.Fatalf("invalid trail: %d %d %v", n, used, err)
	}
	n, used, err = c.Decode(out[:5], []byte{0x82, 0xf5}, true)
	if err != ErrShortDst || n != 0 || used != 0 {
		t.Fatalf("short pair output: %d %d %v", n, used, err)
	}
}
