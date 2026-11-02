// Package textcodecs provides streaming text encoding conversion.
package textcodecs

import (
	"errors"
	"unicode/utf8"
)

var (
	ErrShortDst = errors.New("textcodecs: short destination")
	ErrShortSrc = errors.New("textcodecs: incomplete source")
	ErrInvalid  = errors.New("textcodecs: invalid input")
	ErrUnmapped = errors.New("textcodecs: unmapped character")
)

// isUTF8Three recognizes complete, valid three-byte UTF-8 sequences.
func isUTF8Three(src []byte) bool {
	return len(src) >= 3 && src[0] >= 0xe0 && src[0] < 0xf0 &&
		src[1]&0xc0 == 0x80 && src[2]&0xc0 == 0x80 &&
		(src[0] != 0xe0 || src[1] >= 0xa0) &&
		(src[0] != 0xed || src[1] < 0xa0)
}

// SingleByte is an immutable single-byte encoding. Decode entries contain a
// Unicode scalar value plus one; zero denotes an unmapped byte. Encode contains
// round-trip mappings only. Construct values before sharing them between goroutines.
type SingleByte struct {
	DecodeTable [256]uint32
	EncodeTable map[rune]byte
}

// Decode converts src to UTF-8. No input is consumed for a character that does
// not fit in dst or is unmapped. The returned counts refer to this call.
func (c *SingleByte) Decode(dst, src []byte) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		v := c.DecodeTable[src[nSrc]]
		if v == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r := rune(v - 1)
		n := utf8.RuneLen(r)
		if n < 0 {
			return nDst, nSrc, ErrInvalid
		}
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc++
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to the encoding. When atEOF is false, an incomplete
// UTF-8 sequence is left unconsumed and reported as ErrShortSrc; callers must
// retain that suffix for the next call. Invalid UTF-8 is never replaced.
func (c *SingleByte) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !utf8.FullRune(src[nSrc:]) && !atEOF {
			return nDst, nSrc, ErrShortSrc
		}
		r, n := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && n == 1 {
			return nDst, nSrc, ErrInvalid
		}
		b, ok := c.EncodeTable[r]
		if !ok {
			return nDst, nSrc, ErrUnmapped
		}
		if nDst == len(dst) {
			return nDst, nSrc, ErrShortDst
		}
		dst[nDst] = b
		nDst++
		nSrc += n
	}
	return nDst, nSrc, nil
}
