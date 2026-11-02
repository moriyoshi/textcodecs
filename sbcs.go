package textcodecs

import (
	_ "embed"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:generate go run ./cmd/gensbcs
//go:embed data/icu/sbcs.dat
var sbcsData string

type sbcsEntry struct {
	name          string
	offset, count int
	ascii         bool
}

// SingleByteCodec converts one of the bundled single-byte encodings.
// It is immutable and can be used concurrently. Source and destination must
// not overlap. SingleByteNames returns the supported converter names.
type SingleByteCodec struct{ entry *sbcsEntry }

// LookupSingleByte finds a bundled codec by its UCM file name without the
// .ucm suffix. Names are matched case-insensitively; aliases are not yet
// resolved. An unknown name returns false.
func LookupSingleByte(name string) (SingleByteCodec, bool) {
	name = strings.ToLower(name)
	i := sort.Search(len(sbcsEntries), func(i int) bool {
		return strings.Compare(strings.ToLower(sbcsEntries[i].name), name) >= 0
	})
	if i == len(sbcsEntries) || !strings.EqualFold(sbcsEntries[i].name, name) {
		return SingleByteCodec{}, false
	}
	return SingleByteCodec{entry: &sbcsEntries[i]}, true
}

// SingleByteNames returns the names accepted by LookupSingleByte.
func SingleByteNames() []string {
	names := make([]string, len(sbcsEntries))
	for i := range sbcsEntries {
		names[i] = sbcsEntries[i].name
	}
	return names
}

// Decode converts to UTF-8, leaving an unmapped byte or a character that does
// not fit in dst unconsumed. The codec is stateless between calls.
func (c SingleByteCodec) Decode(dst, src []byte) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		if e.ascii && src[nSrc] < 0x80 && nDst < len(dst) {
			n := copyASCII(dst[nDst:], src[nSrc:])
			nDst += n
			nSrc += n
			if nSrc == len(src) {
				break
			}
		}
		p := e.offset + int(src[nSrc])*2
		v := uint16(sbcsData[p]) | uint16(sbcsData[p+1])<<8
		if v == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r := rune(v - 1)
		n := utf8.RuneLen(r)
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc++
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to the encoding. With atEOF false, callers must retain
// the unconsumed suffix after ErrShortSrc. Unmapped default-ignorable code
// points are consumed without output, matching ICU 74.2 conversion behavior.
func (c SingleByteCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		if e.ascii && src[nSrc] < 0x80 && nDst < len(dst) {
			n := copyASCII(dst[nDst:], src[nSrc:])
			nDst += n
			nSrc += n
			if nSrc == len(src) {
				break
			}
		}
		if !atEOF && !utf8.FullRune(src[nSrc:]) {
			return nDst, nSrc, ErrShortSrc
		}
		r, n := utf8.DecodeRune(src[nSrc:])
		if r == utf8.RuneError && n == 1 {
			return nDst, nSrc, ErrInvalid
		}
		i := sort.Search(e.count, func(i int) bool {
			p := e.offset + 512 + 3*i
			return rune(uint16(sbcsData[p])|uint16(sbcsData[p+1])<<8) >= r
		})
		if i == e.count {
			if icuDefaultIgnorable(r) {
				nSrc += n
				continue
			}
			return nDst, nSrc, ErrUnmapped
		}
		p := e.offset + 512 + 3*i
		key := rune(uint16(sbcsData[p]) | uint16(sbcsData[p+1])<<8)
		if key != r {
			if icuDefaultIgnorable(r) {
				nSrc += n
				continue
			}
			return nDst, nSrc, ErrUnmapped
		}
		if nDst == len(dst) {
			return nDst, nSrc, ErrShortDst
		}
		dst[nDst] = sbcsData[p+2]
		nDst++
		nSrc += n
	}
	return nDst, nSrc, nil
}
