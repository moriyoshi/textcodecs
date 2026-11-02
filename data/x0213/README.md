# Project X0213 mapping

These tables are published by
[Project X0213](https://x0213.org/codetable/index.en.html):

- [EUC-JIS-2004](https://x0213.org/codetable/euc-jis-2004-std.txt):
  `euc-jis-2004-std.txt`, SHA-256
  `efcf5545f25abc359796ecebededdfb13b44d3cfe4f9c82aa42e3b9aae80c281`.
- [Shift_JIS-2004](https://x0213.org/codetable/sjis-0213-2004-std.txt):
  `sjis-0213-2004-std.txt`, SHA-256
  `0def02d3e7a05b126318338a4b71cf30d281816ef6b340c74bd3e906df1ee21a`.

Both files are dated 3 May 2009. Their headers contain copyright notices and
permit use, modification, and distribution. Project X0213 notes that its
volunteer tables are not officially authorized standards mappings.

The wide-codec generator reads these files along with the ICU UCMs. It generates
compact tables for all 11,456 EUC mappings and 11,424 Shift_JIS mappings,
including 25 Unicode scalar pairs in each. `go test` checks each mapping in
both directions against its source. The ICU sync command does not update the
Project X0213 sources.

The Shift_JIS table uses the JIS X 0201 Roman set: single bytes `5C` and `7E`
map to U+00A5 and U+203E. U+005C and U+007E have separate two-byte mappings.

The source assigns single bytes `8E` and `8F` to U+008E and U+008F, and also
uses them as prefixes for multibyte sequences. The decoder prefers a complete
valid shift sequence. When a prefix is followed by an invalid continuation,
or is incomplete at end of input, it decodes the prefix as the corresponding
control and processes any remaining bytes separately. When more input may
follow, an incomplete prefix returns `ErrShortSrc`. This resolves decoding
but cannot make every byte sequence containing a standalone control round-trip
unambiguously: a subsequent byte sequence can form a valid shift sequence.
