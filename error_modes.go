package textcodecs

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrorMode determines what to do with invalid or unmapped input.
type ErrorMode uint8

const (
	// ErrorStrict stops at the offending character without consuming it.
	ErrorStrict ErrorMode = iota
	// ErrorReplace substitutes the configured replacement character.
	ErrorReplace
	// ErrorIgnore discards the offending character.
	ErrorIgnore
)

var (
	// ErrUnknownEncoding indicates that no bundled mapping has the given name.
	ErrUnknownEncoding = errors.New("textcodecs: unknown encoding")
	// ErrInvalidOptions indicates an invalid error mode or replacement character.
	ErrInvalidOptions = errors.New("textcodecs: invalid error options")
)

// ErrorOptions configures error handling independently for both directions.
// Empty replacement strings select '?' for encoding and U+FFFD for decoding.
// Nonempty replacements must each contain exactly one valid Unicode scalar.
// The encoding replacement must be representable by the selected converter.
type ErrorOptions struct {
	EncodeMode, DecodeMode               ErrorMode
	EncodeReplacement, DecodeReplacement string
}

type configuredCodec struct {
	codec                                Codec
	encodeMode, decodeMode               ErrorMode
	encodeReplacement, decodeReplacement string
}

// OpenWithOptions opens a converter with independently configured error modes.
// Open remains equivalent to strict handling in both directions.
func OpenWithOptions(name string, options ErrorOptions) (Codec, error) {
	codec, ok := Open(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownEncoding, name)
	}
	if options.EncodeMode > ErrorIgnore || options.DecodeMode > ErrorIgnore {
		return nil, fmt.Errorf("%w: unknown mode", ErrInvalidOptions)
	}
	enc, dec := options.EncodeReplacement, options.DecodeReplacement
	if enc == "" {
		enc = "?"
	}
	if dec == "" {
		dec = "\ufffd"
	}
	if options.EncodeMode == ErrorReplace && (!utf8.ValidString(enc) || utf8.RuneCountInString(enc) != 1) {
		return nil, fmt.Errorf("%w: encoding replacement must be one Unicode scalar", ErrInvalidOptions)
	}
	if options.DecodeMode == ErrorReplace && (!utf8.ValidString(dec) || utf8.RuneCountInString(dec) != 1) {
		return nil, fmt.Errorf("%w: decoding replacement must be one Unicode scalar", ErrInvalidOptions)
	}
	c := &configuredCodec{codec: codec, encodeMode: options.EncodeMode, decodeMode: options.DecodeMode, encodeReplacement: enc, decodeReplacement: dec}
	if options.EncodeMode == ErrorReplace {
		_, _, err := c.replacement()
		if err != nil {
			return nil, fmt.Errorf("%w: encoding replacement %q is not representable in %s: %v", ErrInvalidOptions, enc, name, err)
		}
	}
	return c, nil
}

func (c *configuredCodec) Reset() { c.codec.Reset() }

// cloneEncoding keeps replacement attempts transactional for stateful codecs.
func cloneEncoding(codec Codec) (Codec, func()) {
	switch c := codec.(type) {
	case *StatefulCodec:
		clone := *c
		return &clone, func() { *c = clone }
	case *WideShiftCodec:
		clone := *c
		return &clone, func() { *c = clone }
	default:
		return codec, func() {}
	}
}

// replacement encodes a replacement using a copy of the encoder state. A
// possible pair prefix is retried at EOF, which may flush a shift state.
func (c *configuredCodec) replacement() ([]byte, func(), error) {
	var buf [64]byte
	clone, commit := cloneEncoding(c.codec)
	n, used, err := clone.Encode(buf[:], []byte(c.encodeReplacement), false)
	if err == ErrShortSrc {
		clone, commit = cloneEncoding(c.codec)
		n, used, err = clone.Encode(buf[:], []byte(c.encodeReplacement), true)
	}
	if err != nil {
		return nil, nil, err
	}
	if used != len(c.encodeReplacement) || n == 0 {
		return nil, nil, ErrUnmapped
	}
	return append([]byte(nil), buf[:n]...), commit, nil
}

func (c *configuredCodec) Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for {
		d, s, e := c.codec.Encode(dst[nDst:], src[nSrc:], atEOF)
		nDst, nSrc = nDst+d, nSrc+s
		if e != ErrInvalid && e != ErrUnmapped || c.encodeMode == ErrorStrict {
			return nDst, nSrc, e
		}
		if nSrc >= len(src) {
			return nDst, nSrc, e
		}
		if c.encodeMode == ErrorReplace {
			replacement, commit, err := c.replacement()
			if err != nil {
				return nDst, nSrc, fmt.Errorf("%w: encoding replacement: %v", ErrInvalidOptions, err)
			}
			if len(dst)-nDst < len(replacement) {
				return nDst, nSrc, ErrShortDst
			}
			nDst += copy(dst[nDst:], replacement)
			commit()
		}
		_, size := utf8.DecodeRune(src[nSrc:])
		nSrc += size
	}
}

func (c *configuredCodec) Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for {
		d, s, e := c.codec.Decode(dst[nDst:], src[nSrc:], atEOF)
		nDst, nSrc = nDst+d, nSrc+s
		if e != ErrInvalid && e != ErrUnmapped || c.decodeMode == ErrorStrict {
			return nDst, nSrc, e
		}
		if nSrc >= len(src) {
			return nDst, nSrc, e
		}
		if c.decodeMode == ErrorReplace {
			if len(dst)-nDst < len(c.decodeReplacement) {
				return nDst, nSrc, ErrShortDst
			}
			nDst += copy(dst[nDst:], c.decodeReplacement)
		}
		width := 1
		if e == ErrUnmapped {
			width = unmappedWidth(c.codec, src[nSrc:])
		}
		nSrc += width
	}
}

// unmappedWidth is called only after the strict decoder recognized a complete
// encoded character whose mapping is absent. Invalid input consumes one byte.
func unmappedWidth(codec Codec, src []byte) int {
	switch c := codec.(type) {
	case registryMultibyte:
		if len(src) >= 2 && mbcs2Data[c.entry.state+int(src[0])] >= 2 {
			return 2
		}
	case registryWide:
		state := 0
		for i, b := range src {
			next := mbcsWideData[c.entry.state+state*256+int(b)]
			if next == 1 {
				return i + 1
			}
			if next == 0 || i+1 >= c.entry.width {
				break
			}
			state = int(next) - 2
		}
	case *StatefulCodec:
		if c.decodeShift && len(src) >= 2 {
			return 2
		}
	case *WideShiftCodec:
		if (!c.entry.stateful || c.decodeShift) && len(src) >= 2 {
			return 2
		}
	}
	return 1
}
