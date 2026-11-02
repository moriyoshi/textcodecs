package textcodecs

import (
	"sort"
	"unicode/utf8"
)

//go:generate go run ./cmd/gengsm

type gsmMapping struct {
	r    rune
	code uint16
}

// GSMCodec converts ICU's GSM 03.38 encoding. It is stateless and safe for
// concurrent use. Source and destination must not overlap.
type GSMCodec struct{}

// GSM is the ICU gsm-03.38-2009 converter.
var GSM GSMCodec

// Decode converts GSM bytes to UTF-8. A trailing escape is retained until the
// next call, unless atEOF is true, when its reverse fallback is used.
func (GSMCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		b := src[nSrc]
		if b >= 128 {
			return nDst, nSrc, ErrInvalid
		}
		v := gsmSingle[b]
		count := 1
		if b == 0x1b {
			if len(src)-nSrc == 1 && !atEOF {
				return nDst, nSrc, ErrShortSrc
			}
			if len(src)-nSrc >= 2 && src[nSrc+1] < 128 {
				if extension := gsmExtension[src[nSrc+1]]; extension != 0 {
					v = extension
					count = 2
				}
			}
		}
		if v == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r := rune(v - 1)
		n := utf8.RuneLen(r)
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc += count
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to GSM. An incomplete UTF-8 suffix is left unconsumed
// with ErrShortSrc unless atEOF is true. ICU default-ignorable characters are
// consumed without output when not otherwise mapped.
func (GSMCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		if !atEOF && !utf8.FullRune(src[nSrc:]) {
			return nDst, nSrc, ErrShortSrc
		}
		r, n := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && n == 1 {
			return nDst, nSrc, ErrInvalid
		}
		i := sort.Search(len(gsmEncode), func(i int) bool { return gsmEncode[i].r >= r })
		if i == len(gsmEncode) || gsmEncode[i].r != r {
			if icuDefaultIgnorable(r) {
				nSrc += n
				continue
			}
			return nDst, nSrc, ErrUnmapped
		}
		v := gsmEncode[i].code - 1
		need := 1
		if v > 255 {
			need = 2
		}
		if len(dst)-nDst < need {
			return nDst, nSrc, ErrShortDst
		}
		if need == 1 {
			dst[nDst] = byte(v)
		} else {
			dst[nDst] = byte(v >> 8)
			dst[nDst+1] = byte(v)
		}
		nDst += need
		nSrc += n
	}
	return nDst, nSrc, nil
}
