package textcodecs

import (
	_ "embed"
	"math/bits"
	"sort"
	"strings"
	"unicode/utf8"
)

//go:generate go run ./cmd/genmbcswide
//go:embed data/icu/mbcswide.dat
var mbcsWideData string

type mbcsWideEntry struct {
	name                                                                        string
	state, shortDecode, decodeIndex, bmpEncode, encodeIndex, encodePages, width int
	pairDecode, pairEncode, pairCount                                           int
	ascii, gb18030, shiftControls                                               bool
}

// WideMultibyteCodec converts a bundled mapping with up to four encoded bytes,
// supplementary characters, and, where defined, Unicode sequences of up to five scalars.
// It is stateless and can be shared between goroutines. Source and destination
// must not overlap.
type WideMultibyteCodec struct{ entry *mbcsWideEntry }

// LookupWideMultibyte resolves a bundled mapping name case-insensitively.
func LookupWideMultibyte(name string) (WideMultibyteCodec, bool) {
	name = strings.ToLower(name)
	i := sort.Search(len(mbcsWideEntries), func(i int) bool { return strings.ToLower(mbcsWideEntries[i].name) >= name })
	if i == len(mbcsWideEntries) || !strings.EqualFold(mbcsWideEntries[i].name, name) {
		return WideMultibyteCodec{}, false
	}
	return WideMultibyteCodec{entry: &mbcsWideEntries[i]}, true
}

// WideMultibyteNames lists the names accepted by LookupWideMultibyte.
func WideMultibyteNames() []string {
	names := make([]string, len(mbcsWideEntries))
	for i := range mbcsWideEntries {
		names[i] = mbcsWideEntries[i].name
	}
	return names
}
func wide16(off int) uint16 {
	s := mbcsWideData[off : off+2]
	return uint16(s[0]) | uint16(s[1])<<8
}
func wide24(off int) uint32 {
	return uint32(mbcsWideData[off]) | uint32(mbcsWideData[off+1])<<8 | uint32(mbcsWideData[off+2])<<16
}
func wide32(off int) uint32 {
	s := mbcsWideData[off : off+4]
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}
func wide64(off int) uint64 {
	s := mbcsWideData[off : off+8]
	return uint64(s[0]) | uint64(s[1])<<8 | uint64(s[2])<<16 | uint64(s[3])<<24 |
		uint64(s[4])<<32 | uint64(s[5])<<40 | uint64(s[6])<<48 | uint64(s[7])<<56
}

// wideRank returns the index of a set bit in a 256-bit group bitmap. The four
// bytes following the bitmap store the number of set bits before each word.
func wideRank(off int, value byte) (int, bool) {
	word := int(value) >> 6
	bit := uint(value & 63)
	mask := wide64(off + word*8)
	if mask&(uint64(1)<<bit) == 0 {
		return 0, false
	}
	rank := int(mbcsWideData[off+32+word]) + bits.OnesCount64(mask&((uint64(1)<<bit)-1))
	return rank, true
}

// Decode converts encoded bytes to UTF-8. Incomplete sequences are left
// unconsumed with ErrShortSrc unless atEOF is true. Invalid and unmapped
// sequences and characters that do not fit in dst are left unconsumed.
// Multi-scalar mappings are emitted atomically.
func (c WideMultibyteCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
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
		first := src[nSrc]
		key := uint32(first) << 24
		count := 1
		next := mbcsWideData[e.state+int(first)]
		if next == 0 {
			return nDst, nSrc, ErrInvalid
		}
		for next != 1 {
			if count >= e.width {
				return nDst, nSrc, ErrInvalid
			}
			if nSrc+count >= len(src) {
				if !atEOF {
					return nDst, nSrc, ErrShortSrc
				}
				if e.shiftControls && (first == 0x8e || first == 0x8f) {
					key, count = uint32(first)<<24, 1
					break
				}
				return nDst, nSrc, ErrInvalid
			}
			b := src[nSrc+count]
			key |= uint32(b) << uint(24-8*count)
			count++
			next = mbcsWideData[e.state+int(next-2)*256+int(b)]
			if next == 0 {
				if e.shiftControls && (first == 0x8e || first == 0x8f) {
					key, count = uint32(first)<<24, 1
					break
				}
				return nDst, nSrc, ErrInvalid
			}
		}
		var r rune
		found := false
		if key&0xffff == 0 {
			pair := int(key >> 16)
			block := decodeBlock(mbcsWideData, e.shortDecode, pair)
			if block != 0 {
				value := wide24(wideSharedShortDecode + (int(block-1)*32+(pair&31))*3)
				if value != 0 {
					r, found = rune(value-1), true
				}
			}
		} else if group := wide16(e.decodeIndex + int(key>>24)*2); group != 0 {
			meta := wideSharedDecodeMeta + int(group-1)*8
			off, size := wideSharedDecodeData+int(wide32(meta)), int(wide32(meta+4))/6
			low := key & 0xffffff
			i := sort.Search(size, func(i int) bool { return wide24(off+i*6) >= low })
			if i < size && wide24(off+i*6) == low {
				r = rune(wide24(off+i*6+3) - 1)
				found = true
			}
		}
		if !found && e.gb18030 && count == 4 {
			r, found = gbDecode(key)
		}
		if !found {
			return nDst, nSrc, ErrUnmapped
		}
		var sequence [5]rune
		sequence[0] = r
		if r >= 0x110000 {
			p := e.pairDecode + int(r-0x110000)*20
			for i := range sequence {
				sequence[i] = rune(wide32(p + i*4))
			}
		}
		n := 0
		for i, r := range sequence {
			if i != 0 && r == 0 {
				break
			}
			n += utf8.RuneLen(r)
		}
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		for i, r := range sequence {
			if i != 0 && r == 0 {
				break
			}
			nDst += utf8.EncodeRune(dst[nDst:], r)
		}
		nSrc += count
	}
	return nDst, nSrc, nil
}

// Encode converts UTF-8 to the encoding. Incomplete UTF-8, or a scalar that
// could begin a multi-scalar mapping, is left unconsumed with ErrShortSrc when
// more input is needed. Unmapped ICU default-ignorable code points are consumed
// without output.
func (c WideMultibyteCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
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
		var key uint32
		length := 0
		if e.pairCount != 0 {
			index := sort.Search(e.pairCount, func(i int) bool { return wide32(e.pairEncode+i*25) >= uint32(r) })
			matched, pending := 0, false
			for i := index; i < e.pairCount; i++ {
				p := e.pairEncode + i*25
				if wide32(p) != uint32(r) {
					break
				}
				used, match := n, true
				for j := 1; j < 5; j++ {
					want := rune(wide32(p + j*4))
					if want == 0 {
						break
					}
					tail := remaining[used:]
					if len(tail) == 0 || !utf8.FullRune(tail) {
						if !atEOF {
							pending = true
						}
						match = false
						break
					}
					got, size := utf8.DecodeRune(tail)
					if got != want || (got == utf8.RuneError && size == 1) {
						match = false
						break
					}
					used += size
				}
				if match && used > matched {
					key, length, matched = wide32(p+20), int(mbcsWideData[p+24]), used
				}
			}
			if pending {
				return nDst, nSrc, ErrShortSrc
			}
			if matched != 0 {
				n = matched
			}
		}
		if length == 0 && r < 0x10000 {
			block := wide32(e.bmpEncode + int(r>>6)*4)
			if block != 0 {
				off := wideSharedBMPEncodeData + int(block-1)
				mask := wide64(off)
				bit := uint(r) & 63
				if mask&(uint64(1)<<bit) != 0 {
					rank := bits.OnesCount64(mask & ((uint64(1) << bit) - 1))
					p := off + 8 + rank*5
					key, length = wide32(p), int(mbcsWideData[p+4])
				}
			}
		} else if length == 0 {
			page := wide16(e.encodeIndex + int(r>>16)*2)
			var group uint16
			if page != 0 {
				group = wide16(e.encodePages + (int(page-1)*256+int((r>>8)&255))*2)
			}
			if group != 0 {
				meta := wideSharedEncodeMeta + int(group-1)*8
				off := wideSharedEncodeData + int(wide32(meta))
				if rank, ok := wideRank(off, byte(r)); ok {
					p := off + 36 + rank*5
					key = wide32(p)
					length = int(mbcsWideData[p+4])
				}
			}
		}
		if length == 0 && e.gb18030 {
			if v, ok := gbEncode(r); ok {
				key = v
				length = 4
			}
		}
		if length == 0 {
			if icuDefaultIgnorable(r) {
				nSrc += n
				continue
			}
			return nDst, nSrc, ErrUnmapped
		}
		if len(dst)-nDst < length {
			return nDst, nSrc, ErrShortDst
		}
		for i := 0; i < length; i++ {
			dst[nDst+i] = byte(key >> uint(24-8*i))
		}
		nDst += length
		nSrc += n
	}
	return nDst, nSrc, nil
}
