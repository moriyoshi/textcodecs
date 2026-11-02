package textcodecs

import (
	"testing"
	"unicode/utf8"
)

func TestIsUTF8Three(t *testing.T) {
	var input [3]byte
	for first := 0; first < 256; first++ {
		input[0] = byte(first)
		for second := 0; second < 256; second++ {
			input[1] = byte(second)
			for third := 0; third < 256; third++ {
				input[2] = byte(third)
				_, size := utf8.DecodeRune(input[:])
				want := size == 3
				if got := isUTF8Three(input[:]); got != want {
					t.Fatalf("input %x: got %t, want %t", input, got, want)
				}
			}
		}
	}
	for _, input := range [][]byte{nil, {0xe3}, {0xe3, 0x81}} {
		if isUTF8Three(input) {
			t.Fatalf("accepted incomplete input %x", input)
		}
	}
}
