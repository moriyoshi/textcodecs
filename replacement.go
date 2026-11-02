package textcodecs

import "unicode/utf8"

// DecodeReplace decodes using U+FFFD for invalid or unmapped input. An invalid
// sequence consumes one byte; an unassigned two-byte sequence consumes both
// bytes. Incomplete input is left unconsumed unless atEOF is true.
func (c MultibyteCodec) DecodeReplace(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		d, s, e := c.Decode(dst[nDst:], src[nSrc:], atEOF)
		nDst += d
		nSrc += s
		if e == nil || e == ErrShortDst || e == ErrShortSrc {
			return nDst, nSrc, e
		}
		if len(dst)-nDst < 3 {
			return nDst, nSrc, ErrShortDst
		}
		dst[nDst], dst[nDst+1], dst[nDst+2] = 0xef, 0xbf, 0xbd
		nDst += 3
		if e == ErrUnmapped && mbcs2Data[c.entry.state+int(src[nSrc])] >= 2 {
			nSrc += 2
		} else {
			nSrc++
		}
	}
	return nDst, nSrc, nil
}

// EncodeReplace encodes UTF-8, replacing each unmapped scalar or invalid UTF-8
// byte with the UCM's <subchar> byte sequence. Incomplete input is left
// unconsumed unless atEOF is true. This method always uses <subchar>, including
// for UCMs that additionally define <subchar1>.
func (c MultibyteCodec) EncodeReplace(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for nSrc < len(src) {
		d, s, e := c.Encode(dst[nDst:], src[nSrc:], atEOF)
		nDst += d
		nSrc += s
		if e == nil || e == ErrShortDst || e == ErrShortSrc {
			return nDst, nSrc, e
		}
		n := int(c.entry.subLen)
		if len(dst)-nDst < n {
			return nDst, nSrc, ErrShortDst
		}
		if n == 1 {
			dst[nDst] = byte(c.entry.subchar)
		} else {
			dst[nDst] = byte(c.entry.subchar >> 8)
			dst[nDst+1] = byte(c.entry.subchar)
		}
		nDst += n
		_, used := utf8.DecodeRune(src[nSrc:])
		nSrc += used
	}
	return nDst, nSrc, nil
}
