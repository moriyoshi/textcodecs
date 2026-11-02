package textcodecs

import (
	_ "embed"
	"encoding/binary"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:generate go run ./cmd/genebcdicwide
//go:embed data/icu/ebcdicwide.dat
var ebcdicWideData string

type ebcdicWideEntry struct {
	name                                                                           string
	state, single, index, decodePairs, encode, encodeCount, encodePairs, pairCount int
	stateful                                                                       bool
}

func ew16(p int) uint16 { return binary.LittleEndian.Uint16([]byte(ebcdicWideData[p : p+2])) }
func ew32(p int) uint32 { return binary.LittleEndian.Uint32([]byte(ebcdicWideData[p : p+4])) }

// WideShiftCodec handles ICU stateful EBCDIC and DBCS converters with
// supplementary and two-scalar Unicode mappings. Each instance owns separate
// encoding and decoding states and must not be shared concurrently.
type WideShiftCodec struct {
	entry                    *ebcdicWideEntry
	encodeShift, decodeShift bool
}

// OpenWideShift opens a converter by its ICU UCM name without .ucm.
func OpenWideShift(name string) (*WideShiftCodec, bool) {
	name = strings.ToLower(name)
	i := sort.Search(len(ebcdicWideEntries), func(i int) bool { return strings.ToLower(ebcdicWideEntries[i].name) >= name })
	if i == len(ebcdicWideEntries) || !strings.EqualFold(ebcdicWideEntries[i].name, name) {
		return nil, false
	}
	return &WideShiftCodec{entry: &ebcdicWideEntries[i]}, true
}

// WideShiftNames lists converter names accepted by OpenWideShift.
func WideShiftNames() []string {
	names := make([]string, len(ebcdicWideEntries))
	for i := range ebcdicWideEntries {
		names[i] = ebcdicWideEntries[i].name
	}
	return names
}

// Reset returns both conversion directions to their initial states.
func (c *WideShiftCodec) Reset() { c.encodeShift = false; c.decodeShift = false }

// Decode converts to UTF-8, maintaining shift state across calls. Input that
// forms an incomplete character is left unconsumed, and decoded pairs of
// Unicode scalars are emitted atomically.
func (c *WideShiftCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		b := src[nSrc]
		if e.stateful && b == 0x0e {
			c.decodeShift = true
			nSrc++
			continue
		}
		if e.stateful && b == 0x0f {
			c.decodeShift = false
			nSrc++
			continue
		}
		double := !e.stateful || c.decodeShift
		count := 1
		var value uint32
		if !double {
			if ebcdicWideData[e.state+int(b)] == 0 {
				return nDst, nSrc, ErrInvalid
			}
			value = ew32(e.single + int(b)*4)
		} else {
			initial := 0
			if e.stateful {
				initial = 1
			}
			state := ebcdicWideData[e.state+initial*256+int(b)]
			if state < 2 {
				return nDst, nSrc, ErrInvalid
			}
			if len(src)-nSrc < 2 {
				if !atEOF {
					return nDst, nSrc, ErrShortSrc
				}
				return nDst, nSrc, ErrInvalid
			}
			trail := src[nSrc+1]
			if ebcdicWideData[e.state+int(state-2)*256+int(trail)] != 1 {
				return nDst, nSrc, ErrInvalid
			}
			count = 2
			pair := int(b)<<8 | int(trail)
			block := ew16(e.index + (pair>>5)*2)
			if block != 0 {
				value = ew32(ebcdicWideBlocks + (int(block-1)*32+(pair&31))*4)
			}
		}
		if value == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r1, r2 := rune(0), rune(0)
		if value >= 0x110001 {
			p := e.decodePairs + int(value-0x110001)*8
			r1 = rune(ew32(p))
			r2 = rune(ew32(p + 4))
		} else {
			r1 = rune(value - 1)
		}
		need := utf8.RuneLen(r1)
		if r2 != 0 {
			need += utf8.RuneLen(r2)
		}
		if len(dst)-nDst < need {
			return nDst, nSrc, ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r1)
		if r2 != 0 {
			nDst += utf8.EncodeRune(dst[nDst:], r2)
		}
		nSrc += count
	}
	if atEOF {
		c.decodeShift = false
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 into the selected encoding. When a scalar can start a
// two-scalar mapping, it is left unconsumed with ErrShortSrc until a following
// scalar or EOF determines the mapping. A final shift-in byte is emitted on EOF.
func (c *WideShiftCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		remaining := src[nSrc:]
		if !atEOF && !utf8.FullRune(remaining) {
			return nDst, nSrc, ErrShortSrc
		}
		r, n := utf8.DecodeRune(remaining)
		if r == utf8.RuneError && n == 1 {
			return nDst, nSrc, ErrInvalid
		}
		var value uint16
		pairIndex := sort.Search(e.pairCount, func(i int) bool { return ew32(e.encodePairs+i*10) >= uint32(r) })
		if pairIndex < e.pairCount && ew32(e.encodePairs+pairIndex*10) == uint32(r) {
			tail := remaining[n:]
			if len(tail) == 0 && !atEOF {
				return nDst, nSrc, ErrShortSrc
			}
			if len(tail) > 0 {
				if !atEOF && !utf8.FullRune(tail) {
					return nDst, nSrc, ErrShortSrc
				}
				r2, n2 := utf8.DecodeRune(tail)
				if !(r2 == utf8.RuneError && n2 == 1) {
					i := sort.Search(e.pairCount-pairIndex, func(j int) bool {
						p := e.encodePairs + (pairIndex+j)*10
						first, second := ew32(p), ew32(p+4)
						return first > uint32(r) || (first == uint32(r) && second >= uint32(r2))
					}) + pairIndex
					if i < e.pairCount {
						p := e.encodePairs + i*10
						if ew32(p) == uint32(r) && ew32(p+4) == uint32(r2) {
							value = ew16(p + 8)
							n += n2
						}
					}
				}
			}
		}
		if value == 0 {
			i := sort.Search(e.encodeCount, func(i int) bool { return ew32(e.encode+i*6) >= uint32(r) })
			if i < e.encodeCount && ew32(e.encode+i*6) == uint32(r) {
				value = ew16(e.encode + i*6 + 4)
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
		double := v > 255
		if !e.stateful && !double {
			return nDst, nSrc, ErrInvalid
		}
		need := 1
		if double {
			need = 2
		}
		if e.stateful && double != c.encodeShift {
			need++
		}
		if len(dst)-nDst < need {
			return nDst, nSrc, ErrShortDst
		}
		if e.stateful && double != c.encodeShift {
			if double {
				dst[nDst] = 0x0e
			} else {
				dst[nDst] = 0x0f
			}
			nDst++
			c.encodeShift = double
		}
		if double {
			dst[nDst] = byte(v >> 8)
			dst[nDst+1] = byte(v)
			nDst += 2
		} else {
			dst[nDst] = byte(v)
			nDst++
		}
		nSrc += n
	}
	if atEOF && e.stateful && c.encodeShift {
		if nDst == len(dst) {
			return nDst, nSrc, ErrShortDst
		}
		dst[nDst] = 0x0f
		nDst++
		c.encodeShift = false
	}
	return nDst, nSrc, nil
}
