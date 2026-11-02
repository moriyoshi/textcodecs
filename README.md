# textcodecs

textcodecs is a Go library for converting between UTF-8 and legacy text
encodings using mappings from ICU, Project X0213, and ebcdic-kana. It provides
streaming converters for 230 encodings, including CP932, EUC-JP, EUC-JIS-2004,
Shift_JIS-2004, EUC-TW, GB18030, GSM 03.38, single-byte code pages, and
stateful EBCDIC encodings. The library is under development.

The mapping tables are included in the Go build; using the library does not
require an ICU installation. On AMD64 and ARM64, supported builds use SIMD for
runs of printable ASCII. Other architectures and builds with `-tags purego` use
portable Go.

The project's own code is licensed under the [BSD 3-Clause License](LICENSE).
Bundled data and adapted material retain their own notices and terms; see
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Getting started

The module currently uses the local module path `textcodecs` and requires Go
1.22 or later. From this repository, import `textcodecs` and open an encoding by
its bundled mapping name:

```go
package main

import (
    "fmt"
    "log"

    "textcodecs"
)

func main() {
    codec, ok := textcodecs.Open("euc-jp-2007")
    if !ok {
        log.Fatal("encoding not found")
    }

    // EUC-JP bytes for あ.
    input := []byte{0xa4, 0xa2}
    output := make([]byte, 16)
    nDst, nSrc, err := codec.Decode(output, input, true)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s (consumed %d bytes)\n", output[:nDst], nSrc)

    codec.Reset()
    nDst, nSrc, err = codec.Encode(output, []byte("あ"), true)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("% x (consumed %d bytes)\n", output[:nDst], nSrc)
}
```

`Decode` converts from the selected encoding to UTF-8; `Encode` converts UTF-8
to the selected encoding. Both return the number of output bytes written, the
number of input bytes consumed, and an error. The example uses a destination
large enough for its input; real applications must handle a short destination.

## Finding an encoding

`EncodingNames()` returns every name accepted by `Open`. Names are
case-insensitive, but ICU aliases are not currently resolved. For example,
use `ibm-943_P15A-2003` for the bundled CP932 mapping; `CP932` is not an alias.
This mapping's control-character mappings differ from some other CP932
implementations.

Use `euc-jis-2004` for the Project X0213 mapping, including its two-scalar
Unicode mappings. The source also assigns standalone `8E` and `8F` to C1
controls. When followed by a valid shift sequence, those bytes take precedence
as shift prefixes; at end of input or before an invalid continuation, they
decode as controls. Consequently, an encoded standalone control followed by a
valid shift continuation is ambiguous and cannot round-trip independently.

Use `shift_jis-2004` for Project X0213's Shift_JIS-2004 mapping. Its single-byte
Roman set maps `5C` to U+00A5 (¥) and `7E` to U+203E (‾); backslash and tilde
have separate two-byte mappings. This differs from the bundled CP932 mapping.

Use `nta-jis-x-0213-sjis` for Shift_JIS-2004 byte forms with the National Tax
Agency JIS mapping, including JIS X 0201 Roman and halfwidth kana. The related
`nta-jis-x-0213-euc` covers only the two JIS planes; see the
[NTA mapping documentation](data/nta/README.md) for coverage and attribution.

The [ebcdic-kana mappings](data/ebcdic-kana/README.md) include `ebcdik`,
`ebcdic-nec`, `ibm-298`, and variants such as
`ibm-290_hitachi-flavored`. Their mappings come from the original project's UCM
files; this library's error behavior differs from that project's decoder.

## Migrating from ebcdic-kana

The original `github.com/moriyoshi/ebcdic-kana/charmap` variables correspond
to these names accepted by `Open` and `OpenWithOptions`:

| Original variable | textcodecs name |
| --- | --- |
| `Ebcdik` | `ebcdik` |
| `EbcdikLfNlAlt` | `ebcdik_lf-nl-alt` |
| `EbcdicNec` | `ebcdic-nec` |
| `EbcdicNecLfNlAlt` | `ebcdic-nec_lf-nl-alt` |
| `Ibm290` | `ibm-8482_P100-1999` |
| `Ibm290Tilde` | `ibm-290_tilde` |
| `Ibm290HitachiFlavored` | `ibm-290_hitachi-flavored` |
| `Ibm298` | `ibm-298` |
| `Ibm298Backslash` | `ibm-298_backslash` |
| `Ibm298HitachiFlavored` | `ibm-298_hitachi-flavored` |

At the bundled ICU revision, `Ibm290` has the same mapping as
`ibm-8482_P100-1999`. It is distinct from `ibm-290_P100-1995`, which does not
map byte `E1` to the euro sign. There is no `ibm-290` alias.

The old `NewDecoder().Bytes(input)` API allocates the output and replaces
undefined bytes with U+FFFD. The equivalent behavior for a complete input is:

```go
func decodeEBCDIC(input []byte) ([]byte, error) {
    codec, err := textcodecs.OpenWithOptions("ibm-8482_P100-1999", textcodecs.ErrorOptions{
        DecodeMode: textcodecs.ErrorReplace,
    })
    if err != nil {
        return nil, err
    }
    output := make([]byte, len(input)*3)
    nDst, _, err := codec.Decode(output, input, true)
    return output[:nDst], err
}
```

For encoding, use `codec.Encode` with UTF-8 input and a caller-provided output
buffer. The default strict mode reports `ErrInvalid` or `ErrUnmapped`, rather
than the original package's `RepertoireError`; use `ErrorReplace` or
`ErrorIgnore` when desired. Unlike the original encoder, textcodecs silently
consumes certain unmapped default-ignorable Unicode characters. For streaming
input, retain the unconsumed suffix indicated by `nSrc` and use `atEOF=true`
only on the final call.

The family-specific functions `SingleByteNames`, `MultibyteNames`,
`WideMultibyteNames`, `StatefulNames`, and `WideShiftNames` list the names
available through their corresponding lookup or open functions. `GSM` also
exposes the GSM 03.38 converter directly.

## Inspecting mappings

`LookupCodecInfo` reports a codec's character byte lengths and whether it uses
shift states. `WalkMappings` visits decoded mappings in byte order, with options
for a byte prefix, a numeric range, and decoder context such as shift controls.
It reports each single encoded character's Unicode output, which may contain
multiple scalars. Returning `false` stops iteration; cancel the context to
interrupt a walk.

```go
info, ok := textcodecs.LookupCodecInfo("ibm-943_P15A-2003")
if ok {
    fmt.Println(info.ByteLengths)
}
err := textcodecs.WalkMappings(context.Background(), "ibm-943_P15A-2003",
    textcodecs.MappingOptions{Length: 2, Prefix: []byte{0x82}},
    func(m textcodecs.DecodedMapping) bool {
        fmt.Printf("% X → %s\n", m.Bytes, m.Text)
        return true
    })
```

The walk describes decoding mappings, including mappings that may not be
selected by the encoder. Lengths exclude state shift controls; pass those as
`MappingOptions.Context` when walking a shifted state.

## Streaming and errors

Pass `atEOF=false` while more input may follow, and `atEOF=true` on the final
call. Keep the same codec for the duration of a stream, especially for stateful
encodings. `Open` creates an independent codec each time; do not use one
instance concurrently. `Reset` returns it to its initial state for reuse.
Source and destination slices must not overlap.

Use the returned counts on every call: process `dst[:nDst]` and advance the
input by `nSrc` bytes. Errors leave the affected character unconsumed:

- `ErrShortDst`: provide more output space and retry with the unconsumed input.
  For stateful encoders, a final shift sequence may require a retry with empty
  input and `atEOF=true`.
- `ErrShortSrc`: retain the unconsumed input, append more bytes, and retry.
- `ErrInvalid`: the input contains an invalid sequence, including an incomplete
  sequence at end of input.
- `ErrUnmapped`: the input character or sequence has no mapping.

Some encodings may need to see an additional Unicode scalar before deciding
how to encode the current one, so callers should preserve all unconsumed input
on `ErrShortSrc`. Like ICU 74.2, encoding silently consumes
certain unmapped default-ignorable Unicode characters, including variation
selectors.

`LookupMultibyte` also offers `DecodeReplace` and `EncodeReplace` on the
returned codec. These replace invalid or unmapped encoded input with U+FFFD,
and replace unmappable Unicode or invalid UTF-8 with the mapping's `<subchar>`
bytes. The encoder uses `<subchar>` even when the mapping also defines
`<subchar1>`.

## Error handling modes

`OpenWithOptions` configures encoding and decoding independently. Each
direction can use `ErrorStrict`, `ErrorReplace`, or `ErrorIgnore`; the zero
value is strict. For example, this restores the replacement behavior of the
old ebcdic-kana decoder while making encoding strict:

```go
codec, err := textcodecs.OpenWithOptions("ebcdik", textcodecs.ErrorOptions{
    DecodeMode: textcodecs.ErrorReplace,
    DecodeReplacement: "�",
})
if err != nil {
    // Handle an unknown encoding or invalid options.
}
_ = codec
```

Replacement defaults to U+FFFD on decode and `?` on encode. Set
`DecodeReplacement` or `EncodeReplacement` to a single Unicode character to
choose another. The encoding replacement must be representable by the chosen
codec; `OpenWithOptions` returns `ErrInvalidOptions` if it is not. Some
encodings do not contain `?`, so they require an explicit encoding replacement.
The character is encoded in the current shift state. In particular, the
default `?` is not necessarily the same as the UCM `<subchar>` used by
`MultibyteCodec.EncodeReplace`.

Replace and ignore modes consume one byte for an invalid encoded sequence and
the entire character for a complete but unmapped encoded sequence. On encode,
they consume one invalid UTF-8 byte or one unmapped Unicode scalar. Incomplete
input is still returned as `ErrShortSrc` when more input may follow; if a
replacement cannot fit, `ErrShortDst` leaves the affected input unconsumed.

## Mapping data and development

The ICU mappings come from ICU's source tree at the commit recorded in
[data/icu/UPSTREAM_REVISION](data/icu/UPSTREAM_REVISION). The mapping data is
covered by [ICU-LICENSE](ICU-LICENSE). EUC-JIS-2004 and Shift_JIS-2004 come
from [Project X0213](data/x0213/README.md), with licenses in the source tables.
The [ebcdic-kana mappings](data/ebcdic-kana/README.md) retain their source
repository's license.
The [NTA JIS mapping data and comparison](data/nta/README.md) are generated
from the National Tax Agency's JIS Reduction Map; the agency requires attribution.
The [PHP mbstring mapping data](data/php-mbstring/README.md) includes distinct
`php-` codecs generated from a pinned PHP source revision, with coverage limits
for stateful variants.

### PHP mbstring attribution and mapping data

The `php-` mappings describe the behavior of
[PHP mbstring's libmbfl](https://github.com/php/php-src/tree/fb193d3df72d1ca3b5ef58ec9e9b6ef5bdf18bef/ext/mbstring/libmbfl),
including work by HappySize, Inc., the PHP internationalization team, and other
libmbfl and PHP contributors. We thank those authors and contributors. The
upstream [libmbfl README](https://github.com/php/php-src/blob/fb193d3df72d1ca3b5ef58ec9e9b6ef5bdf18bef/ext/mbstring/libmbfl/README.md)
and individual source files contain their copyright and license notices.

Our view is that the individual byte-to-Unicode and Unicode-to-byte
relationships extracted from mbstring are functional mapping facts, not
copyrightable expression, and that observing those relationships does not by
itself apply the LGPL to the generated mapping data. Copyright in source code
and any original selection or arrangement of data is a separate matter; the
upstream notices continue to apply to upstream code. The
[U.S. Copyright Office's guidance on compilations](https://www.copyright.gov/register/tx-compilations.html)
distinguishes original compilation authorship from the underlying data.
Database rights and other applicable rules can differ by jurisdiction; this
statement is the project's position, not a determination for every jurisdiction.

Run `go test ./...` to test the library. For information on updating ICU source
data, regenerating the bundled tables, and running ICU differential tests, see
the [ICU data workflow](docs/icu-data-workflow.md).

## Codec browser

Run `go run ./cmd/codec-browser` and open <http://127.0.0.1:8080> to
browse the registry and inspect decoded byte sequences. Filter the codec list,
select a cell for its Unicode code points and Unicode 15.0 character names, and choose a two-, three-, or four-byte table to
scroll through the complete sequence space or jump to a hexadecimal code.
The available lengths depend on the codec. The canvas minimap aligns with the table columns and marks ranges of rows
that contain at least one mapping. Browser history and shareable URLs
retain the codec, prefix, table position, selected character, and display font. Enter a hexadecimal byte prefix
to explore longer sequences, or use the + control on a one-byte cell to
open the next prefix page. For shift-state codecs, select SO in the table to
enter the shifted state; SI returns to the initial state. Each cell decodes its full byte
sequence from the initial state, so a prefix can include shift controls. A cell
is only shown as mapped when the complete sequence produces output without an
error; incomplete sequences may need a longer prefix.

The display font selector offers generic fonts, common Japanese, Korean, and Chinese
system fonts, and a curated set of Noto families. Named system fonts require a
local installation. Noto fonts load from Google Fonts and require a network connection.
Browsers can use fallback fonts for individual glyphs and do not reliably
expose which font rendered each one. The
browser listens only on localhost by default; use `-listen` to change the address.

The browser bundles Unicode 15.0 character names generated from Python’s
`unicodedata` module; the Unicode data license is included in [ICU-LICENSE](ICU-LICENSE).
