package textcodecs

import (
	"sort"
	"strings"
)

// Codec converts between one encoding and UTF-8. On ErrShortSrc, retain the
// unconsumed suffix and provide it again with more input. Reset returns the
// converter to its initial state. An instance must not be used concurrently;
// callers can open separate instances for concurrent streams.
// Source and destination slices must not overlap.
type Codec interface {
	Encode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
	Decode(dst, src []byte, atEOF bool) (nDst, nSrc int, err error)
	Reset()
}

type registrySingle struct{ SingleByteCodec }

func (c registrySingle) Decode(dst, src []byte, _ bool) (int, int, error) {
	return c.SingleByteCodec.Decode(dst, src)
}
func (registrySingle) Reset() {}

type registryMultibyte struct{ MultibyteCodec }

func (registryMultibyte) Reset() {}

type registryWide struct{ WideMultibyteCodec }

func (registryWide) Reset() {}

type registryGSM struct{ GSMCodec }

func (registryGSM) Reset() {}

// Open creates a converter by its bundled mapping name. ICU mappings use their
// UCM filenames without the .ucm suffix; other mappings use their listed names.
// Names are case-insensitive. ICU aliases are not yet resolved. Each call
// returns an independent converter, suitable for one stream at a time.
func Open(name string) (Codec, bool) {
	if strings.EqualFold(name, "gsm-03.38-2009") {
		return registryGSM{}, true
	}
	if c, ok := LookupSingleByte(name); ok {
		return registrySingle{c}, true
	}
	if c, ok := LookupMultibyte(name); ok {
		return registryMultibyte{c}, true
	}
	if c, ok := LookupWideMultibyte(name); ok {
		return registryWide{c}, true
	}
	if c, ok := OpenStateful(name); ok {
		return c, true
	}
	if c, ok := OpenWideShift(name); ok {
		return c, true
	}
	return nil, false
}

// EncodingNames returns every bundled mapping name accepted by Open.
func EncodingNames() []string {
	names := make([]string, 0, len(sbcsEntries)+len(mbcs2Entries)+len(mbcsWideEntries)+len(ebcdicWideEntries)+1)
	names = append(names, SingleByteNames()...)
	names = append(names, MultibyteNames()...)
	names = append(names, WideMultibyteNames()...)
	names = append(names, StatefulNames()...)
	names = append(names, WideShiftNames()...)
	names = append(names, "gsm-03.38-2009")
	sort.Strings(names)
	return names
}
