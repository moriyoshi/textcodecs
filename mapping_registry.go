package textcodecs

import (
	"context"
	"fmt"
	"strings"
)

// CodecInfo describes the byte lengths of decoded character mappings. For
// shift-state codecs, lengths exclude shift controls. Names are registry names,
// not aliases. The returned slice belongs to the caller.
type CodecInfo struct {
	Name        string
	ByteLengths []int
	Stateful    bool
}

// LookupCodecInfo returns mapping metadata for a registered codec. For a
// stateful codec, ByteLengths is the union of lengths across its shift states.
func LookupCodecInfo(name string) (CodecInfo, bool) {
	codec, ok := Open(name)
	if !ok {
		return CodecInfo{}, false
	}
	info := CodecInfo{Name: name}
	for _, canonical := range EncodingNames() {
		if strings.EqualFold(canonical, name) {
			info.Name = canonical
			break
		}
	}
	switch c := codec.(type) {
	case registrySingle:
		info.ByteLengths = []int{1}
	case registryMultibyte, registryGSM:
		info.ByteLengths = []int{1, 2}
	case registryWide:
		var found [5]bool
		e := c.WideMultibyteCodec.entry
		var walk func(int, int)
		walk = func(base, depth int) {
			for b := 0; b < 256; b++ {
				next := mbcsWideData[base+b]
				if next == 1 {
					found[depth] = true
				} else if next > 1 && depth < e.width {
					walk(e.state+int(next-2)*256, depth+1)
				}
			}
		}
		walk(e.state, 1)
		for i := 1; i < len(found); i++ {
			if found[i] {
				info.ByteLengths = append(info.ByteLengths, i)
			}
		}
	default:
		info.Stateful = true
		info.ByteLengths = []int{1, 2}
	}
	return info, true
}

// DecodedMapping is a single encoded character and the Unicode text it decodes
// to. Text can contain multiple Unicode scalars. Bytes is owned by the caller.
type DecodedMapping struct {
	Bytes []byte
	Text  string
}

// MappingOptions limits a walk to byte sequences of Length bytes. Context is
// decoded before each candidate, allowing shift controls or an incomplete
// character prefix. Context may also produce output, which is excluded from
// Text. Prefix restricts the candidates, and Start and End bound their unsigned
// big-endian values to [Start, End). A zero End means the end of the space.
type MappingOptions struct {
	Length     int
	Context    []byte
	Prefix     []byte
	Start, End uint64
}

// WalkMappings visits complete decoded mappings in byte order. It excludes
// sequences whose proper prefix already produces a character, so adjacent
// characters are not reported as one mapping. Returning false from visit stops
// the walk successfully. The context cancels the walk; a codec instance is
// private to the walk and is reset between candidates.
func WalkMappings(ctx context.Context, name string, options MappingOptions, visit func(DecodedMapping) bool) error {
	codec, ok := Open(name)
	if !ok {
		return fmt.Errorf("textcodecs: unknown codec %q", name)
	}
	if ctx == nil || visit == nil {
		return fmt.Errorf("textcodecs: context and visitor are required")
	}
	if options.Length < 1 || options.Length > 4 || len(options.Prefix) > options.Length || len(options.Context) > 64 {
		return fmt.Errorf("textcodecs: invalid mapping options")
	}
	limit := uint64(1) << uint(8*options.Length)
	end := options.End
	if end == 0 {
		end = limit
	}
	if options.Start > end || end > limit {
		return fmt.Errorf("textcodecs: invalid mapping range")
	}
	dst := make([]byte, 512)
	codec.Reset()
	baseline, _, _ := codec.Decode(dst, options.Context, false)
	input := make([]byte, len(options.Context)+options.Length)
	copy(input, options.Context)
	stopped := false
	var walk func(int, uint64) error
	walk = func(depth int, value uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		remaining := options.Length - depth
		first, last := value<<uint(remaining*8), (value+1)<<uint(remaining*8)
		if first >= end || last <= options.Start {
			return nil
		}
		for b := 0; b < 256 && !stopped; b++ {
			if depth < len(options.Prefix) && byte(b) != options.Prefix[depth] {
				continue
			}
			next := value<<8 | uint64(b)
			rem := options.Length - depth - 1
			if next<<uint(rem*8) >= end || (next+1)<<uint(rem*8) <= options.Start {
				continue
			}
			input[len(options.Context)+depth] = byte(b)
			codec.Reset()
			n, consumed, err := codec.Decode(dst, input[:len(options.Context)+depth+1], depth+1 == options.Length)
			if depth+1 == options.Length {
				if err == nil && consumed == len(input) && n > baseline {
					stopped = !visit(DecodedMapping{Bytes: append([]byte(nil), input[len(options.Context):]...), Text: string(dst[baseline:n])})
				}
			} else if n <= baseline && (err == nil || err == ErrShortSrc) {
				if err := walk(depth+1, next); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(0, 0)
}
