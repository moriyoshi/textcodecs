package textcodecs

import (
	"math/bits"
	"sort"
	"strings"
	"unicode/utf8"
)

// StatefulCodec converts a bundled ICU shift-state encoding. Encoding and
// decoding maintain independent shift states. Instances must not be shared
// concurrently; OpenStateful returns a fresh converter for each stream.
type StatefulCodec struct {
	entry                    *mbcs2Entry
	decodeShift, encodeShift bool
}

// OpenStateful opens a bundled stateful converter by UCM filename without .ucm.
func OpenStateful(name string) (*StatefulCodec, bool) {
	name = strings.ToLower(name)
	i := sort.Search(len(mbcs2Entries), func(i int) bool { return strings.ToLower(mbcs2Entries[i].name) >= name })
	if i == len(mbcs2Entries) || !strings.EqualFold(mbcs2Entries[i].name, name) || !mbcs2Entries[i].stateful {
		return nil, false
	}
	return &StatefulCodec{entry: &mbcs2Entries[i]}, true
}

// StatefulNames lists names accepted by OpenStateful.
func StatefulNames() []string {
	names := make([]string, 0)
	for i := range mbcs2Entries {
		if mbcs2Entries[i].stateful {
			names = append(names, mbcs2Entries[i].name)
		}
	}
	return names
}

// Reset returns both conversion directions to the initial single-byte state.
func (c *StatefulCodec) Reset() { c.decodeShift = false; c.encodeShift = false }

// Decode converts encoded bytes to UTF-8 while maintaining shift state between calls.
// Shift bytes may be consumed without producing output. The current state is
// reset after a successful call with atEOF true.
func (c *StatefulCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		b := src[nSrc]
		if b == 0x0e {
			c.decodeShift = true
			nSrc++
			continue
		}
		if b == 0x0f {
			c.decodeShift = false
			nSrc++
			continue
		}
		var value uint16
		count := 1
		if !c.decodeShift {
			if mbcs2Data[e.state+int(b)] == 0 {
				return nDst, nSrc, ErrInvalid
			}
			value = mbcs16(e.single + int(b)*2)
		} else {
			state := mbcs2Data[e.state+256+int(b)]
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
			if mbcs2Data[e.state+int(state-2)*256+int(trail)] != 1 {
				return nDst, nSrc, ErrInvalid
			}
			count = 2
			pair := int(b)<<8 | int(trail)
			block := mbcs16(e.decodeIndex + (pair>>5)*2)
			if block != 0 {
				value = mbcs16(mbcsSharedDecode + (int(block-1)*32+(pair&31))*2)
			}
		}
		if value == 0 {
			return nDst, nSrc, ErrUnmapped
		}
		r := rune(value - 1)
		n := utf8.RuneLen(r)
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		nDst += utf8.EncodeRune(dst[nDst:], r)
		nSrc += count
	}
	if atEOF {
		c.decodeShift = false
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to encoded bytes and writes shift bytes as needed. A
// successful atEOF call emits a final shift-in byte if required and resets the
// encoding state. If the final byte does not fit, retry with empty input and
// atEOF true after ErrShortDst.
func (c *StatefulCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	e := c.entry
	for nSrc < len(src) {
		if !atEOF && !utf8.FullRune(src[nSrc:]) {
			return nDst, nSrc, ErrShortSrc
		}
		r, n := utf8.DecodeRune(src[nSrc:])
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
		double := v > 255
		need := 1
		if double {
			need = 2
		}
		if double != c.encodeShift {
			need++
		}
		if len(dst)-nDst < need {
			return nDst, nSrc, ErrShortDst
		}
		if double != c.encodeShift {
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
	if atEOF && c.encodeShift {
		if nDst == len(dst) {
			return nDst, nSrc, ErrShortDst
		}
		dst[nDst] = 0x0f
		nDst++
		c.encodeShift = false
	}
	return nDst, nSrc, nil
}
