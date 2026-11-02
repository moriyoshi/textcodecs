package textcodecs

import (
	_ "embed"
	"encoding/binary"
	"math/bits"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:generate go run ./cmd/genmbcs2
//go:embed data/icu/mbcs2.dat
var mbcs2Data string

type mbcs2Entry struct {
	name                                    string
	state, single, decodeIndex, encodeIndex int
	subchar                                 uint16
	subLen                                  byte
	ascii, stateful                         bool
}

// MultibyteCodec is an immutable converter for one of the bundled ICU
// multibyte encodings. It can be shared by concurrent callers. Source and
// destination slices must not overlap.
type MultibyteCodec struct{ entry *mbcs2Entry }

// LookupMultibyte finds a codec by UCM filename without its .ucm suffix. The
// match is case-insensitive; ICU aliases are not yet supported.
func LookupMultibyte(name string) (MultibyteCodec, bool) {
	name = strings.ToLower(name)
	i := sort.Search(len(mbcs2Entries), func(i int) bool { return strings.ToLower(mbcs2Entries[i].name) >= name })
	if i == len(mbcs2Entries) || !strings.EqualFold(mbcs2Entries[i].name, name) || mbcs2Entries[i].stateful {
		return MultibyteCodec{}, false
	}
	return MultibyteCodec{entry: &mbcs2Entries[i]}, true
}

// MultibyteNames lists names accepted by LookupMultibyte.
func MultibyteNames() []string {
	names := make([]string, 0, len(mbcs2Entries))
	for i := range mbcs2Entries {
		if !mbcs2Entries[i].stateful {
			names = append(names, mbcs2Entries[i].name)
		}
	}
	return names
}
func mbcs16(off int) uint16 { return binary.LittleEndian.Uint16([]byte(mbcs2Data[off : off+2])) }
func mbcs32(off int) uint32 { return binary.LittleEndian.Uint32([]byte(mbcs2Data[off : off+4])) }
func mbcs64(off int) uint64 { return binary.LittleEndian.Uint64([]byte(mbcs2Data[off : off+8])) }

// decodeBlock identifies a shared 32-entry block; zero denotes an empty block.
func decodeBlock(data string, index, pair int) uint16 {
	p := index + (pair>>5)*2
	return uint16(data[p]) | uint16(data[p+1])<<8
}

// Decode converts to UTF-8. An incomplete byte sequence is unconsumed and
// returns ErrShortSrc unless atEOF is true. Invalid or unmapped sequences and
// characters that do not fit in dst are left unconsumed.
func (c MultibyteCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
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
		b := src[nSrc]
		state := mbcs2Data[e.state+int(b)]
		count := 1
		var value uint16
		if state == 0 {
			return nDst, nSrc, ErrInvalid
		}
		if state == 1 {
			value = mbcs16(e.single + int(b)*2)
		} else {
			if len(src)-nSrc < 2 {
				if !atEOF {
					return nDst, nSrc, ErrShortSrc
				}
				return nDst, nSrc, ErrInvalid
			}
			trail := src[nSrc+1]
			next := mbcs2Data[e.state+int(state-2)*256+int(trail)]
			if next != 1 {
				return nDst, nSrc, ErrInvalid
			}
			count = 2
			pair := int(b)<<8 | int(trail)
			block := decodeBlock(mbcs2Data, e.decodeIndex, pair)
			if block != 0 {
				value = mbcs16(mbcsSharedDecode + (int(block-1)*32+(pair&31))*2)
			}
		}
		if value == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r := uint32(value - 1)
		n := 1
		if r >= 0x800 {
			n = 3
		} else if r >= 0x80 {
			n = 2
		}
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		switch n {
		case 1:
			dst[nDst] = byte(r)
		case 2:
			dst[nDst], dst[nDst+1] = 0xc0|byte(r>>6), 0x80|byte(r&0x3f)
		case 3:
			dst[nDst], dst[nDst+1], dst[nDst+2] = 0xe0|byte(r>>12), 0x80|byte((r>>6)&0x3f), 0x80|byte(r&0x3f)
		}
		nDst += n
		nSrc += count
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to the encoding. An incomplete UTF-8 suffix is left
// unconsumed with ErrShortSrc unless atEOF is true. Unmapped ICU default-
// ignorable characters are consumed without output.
func (c MultibyteCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
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
		remaining := src[nSrc:]
		var r rune
		var n int
		if isUTF8Three(remaining) {
			r = rune(remaining[0]&15)<<12 | rune(remaining[1]&63)<<6 | rune(remaining[2]&63)
			n = 3
		} else {
			if !atEOF && !utf8.FullRune(remaining) {
				return nDst, nSrc, ErrShortSrc
			}
			r, n = utf8.DecodeRune(remaining)
		}
		if r == utf8.RuneError && n == 1 {
			return nDst, nSrc, ErrInvalid
		}
		var value uint16
		if r < 65536 {
			block := mbcs16(e.encodeIndex + (int(r)>>6)*2)
			if block != 0 {
				block--
				bit := uint(r) & 63
				mask := mbcs64(mbcsSharedMask + int(block)*8)
				if mask&(uint64(1)<<bit) != 0 {
					index := int(mbcs32(mbcsSharedOffset+int(block)*4)) + bits.OnesCount64(mask&((uint64(1)<<bit)-1))
					value = mbcs16(mbcsSharedEncode + index*2)
				}
			}
		}
		if value == 0 {
			if icuDefaultIgnorable(r) {
				nSrc += n
				continue
			}
			return nDst, nSrc, ErrUnmapped
		}
		v := value - 1
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
