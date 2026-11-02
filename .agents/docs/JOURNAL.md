# Project journal

Last updated: 2026-11-02

## Chronological project history

The order below follows the project conversation and checked-in artifacts. The
available memory does not establish a separate date for each event; the numbers
are sequence markers, not dates. It also does not preserve reliable benchmark
figures or every intermediate implementation, so do not infer a performance
result from the fact that an approach was investigated.

### 1. Project goal and initial direction

The project began with a request for a SIMD-optimized Go encoding conversion
library whose mapping source is ICU's `icu4c/source/data/mappings` tree. The
user wanted an implementation, not just a plan, and repeatedly directed work
toward completion. The design needed to serve general encoding conversion, not
only a single Japanese code page. Mapping correctness, streaming behavior, and
speed are all requirements; a fast-path optimization must preserve the table's
actual semantics.

### 2. Compact tables and lessons from jntajis-go

The user stressed that a **small mapping representation** matters and asked
for a real inspection of `kenall-inc/jntajis-go`, rather than an assumed summary.
Early attempts to access the public repository encountered environment/network
problems and led to explicit user frustration; checking out and inspecting the
actual source was necessary. The user asked to take useful parts from both
projects rather than replacing one entire design with the other.

The resulting direction is to pre-generate compact packed data, share blocks
and indexes where useful, and query the embedded tables directly. This is not
lazy per-codec expansion at runtime. The current implementations and layout are
in `cmd/genmbcs2`, `cmd/genmbcswide`, `mbcs2.go`, `mbcswide.go`, and the
workflow documentation. Preserve those motivations when adding codecs.

### 3. Removing CP932-specific machinery

During table unification, the user noticed CP932-specific logic inside the
SBCS codec and challenged why a generic component needed that special case.
The user then questioned whether a separate CP932 implementation and
`cp932.go` were still necessary. The direction was to use shared generated
codec machinery and remove the redundant dedicated implementation, while
keeping CP932 behavior covered by tests. The lesson is to justify codec-specific
code by a real semantic requirement, not by its role as an early prototype.

### 4. GB18030 and the source-update pipeline

The user raised GB18030 after the CP932 cleanup. Its four-byte algorithmic
ranges cannot be treated as just an ordinary small explicit lookup table.
`gb18030.go` and `gb18030_test.go` capture the special range handling, while
`cmd/syncucm` checks the pinned upstream algorithmic range rows as part of ICU
updates. The check covers those rows, not every possible change in ICU's
implementation; inspect relevant upstream changes when updating.

The user requested an ICU UCM synchronization script and then asked for it to
be rewritten in Go. `cmd/syncucm` became the Go workflow. The user also asked
for the update and binary-generation pipeline to be written down, resulting
in `docs/icu-data-workflow.md`. That document describes immutable source refs,
source inventory hashes, staging, regeneration, ICU 74.2 oracle tests, and the
fact that replacing multiple files is not a single atomic transaction. The
pinned ICU source commit recorded in the current tree is
`b34cf4d07e71acf06f01b3a1902759609c111e2d`.

### 5. README and an EUC-JP correctness check

The user asked for `README.md` to be a **user-facing** document. Operational
update details belong in the workflow documentation; the main README explains
opening codecs, streaming, error modes, source coverage, and migration.

A concrete correctness requirement was added for EUC-JP: `8E B1` is Single
Shift 2 plus halfwidth Katakana A, U+FF71. Preserve this when changing prefix
handling or optimizing the wide decoder. It is not enough to exercise only
ordinary two-byte JIS plane characters.

### 6. Performance investigations and shared lookup paths

The user asked for performance levers, chose the first proposed lever, and
subsequently asked whether SIMD could improve bitmap-index searching. Both
suggested SIMD approaches were to be tried. The user then asked about the
performance gap between CP932 and EUC-JP and whether the codecs could share an
approach; the shared-path idea was pursued. Further levers were requested.

The durable result is the current combination of generated shared short decode
blocks, sparse bitmap/rank indexes, direct BMP encoding blocks, and ASCII
fast paths. The conversation memory does not include sufficient benchmark
numbers or the final comparative outcome of each SIMD-search experiment; do
not describe an unrecorded candidate as a measured win. Re-benchmark current
code and workloads before reviving or changing a performance technique.

### 7. Project X0213 EUC and Shift_JIS sources

The user requested mappings from Project X0213 and then Shift_JIS mappings as
well. The source files are in `data/x0213`, separately maintained from ICU.
They cover 11,456 EUC-JIS-2004 and 11,424 Shift_JIS-2004 mappings, each with
25 two-scalar Unicode mappings. `x0213_test.go` checks the published mappings
and streaming boundaries.

These are not interchangeable with every other codec called EUC-JP or SJIS.
The X0213 Shift_JIS source uses JIS X 0201 Roman: `5C` -> U+00A5 and `7E` ->
U+203E. Its EUC table assigns standalone `8E` and `8F` as C1 controls as well
as using them as multibyte prefixes, so the decoder's documented precedence
and end-of-input behavior are material. See `data/x0213/README.md`.

### 8. Embedded strings, assembly inspection, and compression

The user questioned storing `mbcsWideData` as a string, asked for assembly
inspection to check whether `[]byte` casts had overhead, and asked about
pointer arithmetic. The explicit decision was to use direct string indexing
for the embedded packed data. The user later proposed zstd or LZ4 compression
for `.dat` files, then dismissed the idea because the observed win was small.
Do not reintroduce decompression or unsafe pointer arithmetic solely on
intuition; establish current size, runtime, complexity, and allocation costs.

### 9. ebcdic-kana consolidation and error handling

The user asked this project to supersede `moriyoshi/ebcdic-kana` by bringing
in its mapping tables. That did not authorize treating every similarly named
mapping as a duplicate. The user subsequently requested independent strict,
replace, and ignore modes for encoding and decoding, with configurable
replacement characters; `OpenWithOptions` implements that contract. Keep
streaming consumption and stateful replacement behavior covered by tests.

The upstream ebcdic-kana `ibm290` mapping was found identical in both
directions to ICU `ibm-8482_P100-1999`. The user explicitly clarified that
they did **not** want unrelated mappings consolidated merely by name. The
separate ICU `ibm-290_P100-1995`, the tilde variant, and Hitachi-flavored
variants retain distinct semantics. The user authorized editing the inherited
UCM filenames and names because this project supersedes the upstream, chose
names such as `ibm-290_hitachi-flavored`, and rejected the ambiguous bare
`ibm-290` alias. A README migration guide explains replacements and behavior.
The ebcdic-kana source revision in the current tree is
`4c740a3af1965e3cf8e4f2d8ec3d58c45705fa2f`.

### 10. NTA spreadsheet reader and mapping comparison

The user requested a port of the jntajis-go XLSX reader, generation of a UCM,
and a comparison against existing mappings. The spreadsheet contains both
source JIS-to-Unicode entries and separate reduction/transliteration rules.
The implementation deliberately generates conversion mappings from the source
columns and records reduction information as comments rather than treating
transliteration as a round trip. The jntajis-go BSD notice is retained in
`data/nta/JNTAJIS-LICENSE`.

The pinned workbook has SHA-256
`61973084e775593964d2bc3358c223bb01481f1b6555634a420682a2d649405b`.
It contains 11,230 mapped positions, including 25 two-scalar primary mappings,
seven alternate Unicode values, and 7,940 rows with reduction information.
The latter number counts rows with reduction information, not necessarily
7,940 changes or non-identity transliterations.

The user drew attention to jntajis-go's Makefile. `make nta-jis-ucm` adapts its
fetch/extract/generate workflow, verifies the archive and workbook hashes, and
uses the archive's real `jissyukutaimap1_0_0.xlsx` filename. It regenerates the
UCMs, comparison reports, and bundled wide tables. The source UCM and report
include NTA attribution, and a `makeconv` check validated the generated UCM.

The comparison in `data/nta/comparison.md` found nine different decoded
positions versus EUC-JIS-2004 and eight versus Shift_JIS-2004; each X0213 codec
has three mapped positions absent from the spreadsheet. ICU EUC-JP and CP932
comparisons were restricted to plane 1 because their extension spaces are not
JIS X 0213 plane 2. The TSV contains individual differences. The user then
asked for the mapping itself to be registered, resulting initially in the
plane-only `nta-jis-x-0213-euc` codec.

### 11. PHP mbstring mappings and direct source extraction

The user asked for an inspection of the local php-src mbstring `libmbfl` tree,
then requested UCM generation. Inspection found that PHP behavior combines C
tables, conversion code, and test overrides; static test tables alone do not
capture preferred encodings, compatibility mappings, or algorithmic areas.
The user recommended a short C extractor using mbstring source files instead
of a PHP script. The final approach compiles `cmd/phpmbucm/testdata/probe.c`
against a configured php-src checkout and feeds its results to a Go UCM
generator; it does not need a PHP runtime to run the probe.

At the recorded php-src revision `fb193d3df72d1ca3b5ef58ec9e9b6ef5bdf18bef`,
the extractor produced 42 UCMs: 23 SBCS and 19 CJK variants. It probes both
directions, including sequences produced by decoding. The generated data is
not a full model of all PHP streaming states or arbitrary Unicode input
sequences; in particular, SoftBank escape states, ISO-2022/HZ, UTF and
transfer encodings, and algorithmic GB18030 variants are outside its scope.
The separate build/redistribution considerations for the probe and upstream
libmbfl source are noted in `THIRD_PARTY_NOTICES.md`.

### 12. PHP registry selection and multi-scalar support

The user asked to register only non-duplicate PHP mappings. Fifteen SBCS UCMs
were identified as exact mapping duplicates of existing registered codecs;
27 distinct PHP mappings were registered under `php-` names. The lists are
`data/php-mbstring/registry-sbcs.txt` and `registry-wide.txt`, while all 42
source UCMs remain for inspection. Avoid inferring that a source file is a
registered codec solely from its presence.

MacJapanese mappings required up to five Unicode scalars per encoded character;
mobile emoji mappings can start with ASCII. The generic wide table and streaming
encoder/decoder were extended rather than introducing a PHP-only special case.
Tests check the registered UCM mappings, a five-scalar MacJapanese example,
ASCII-starting emoji, streaming prefix retention, and atomic decoding.

### 13. Follow-up duplicate investigations

The user specifically asked whether PHP's SJIS and EUC-JP variants duplicated
Project X0213; both JIS-2004 variants were retained after checking actual
behavior. PHP Shift_JIS-2004 decodes `81 5F`/`81 B0` to U+FF3C/U+FF5E,
whereas Project X0213 uses U+005C/U+007E. PHP EUC-JP-2004 uses U+FFE3 and
U+FFE5 at `A1 B1` and `A1 EF`, whereas Project X0213 uses U+203E and U+00A5.
Their encoding choices also differ. Regression tests protect these differences.

The user then asked about Big5, CP850, CP866, CP936, CP950, EUC-CN, EUC-KR,
EUC-TW, and the remaining SBCS mappings. None of those registered PHP variants
was an exact duplicate of the corresponding bundled ICU codec. In particular,
CP936's decoded table matches Windows-936 but its encoder accepts 284 additional
inputs; CP950 differs in preferred bytes for four shared Unicode characters;
and IBM CP850/CP866 differ at three controls. The full comparison scope and
counts are in `data/php-mbstring/README.md`, with focused regression tests.
The SBCS audit compared all byte values and BMP Unicode scalars in both
directions, excluding silently ignored scalars from the definition of a mapping.

### 14. NTA Shift_JIS variant

After the initial plane-only EUC codec, the user requested a Shift_JIS variant
for the NTA mapping. `nta-jis-x-0213-sjis` uses the same NTA JIS positions in
Shift_JIS-2004 byte form and includes JIS X 0201 Roman and halfwidth kana.
It has 11,421 primary mappings: the 11,230 spreadsheet positions plus 191
single-byte Roman/kana mappings. The EUC variant intentionally remains
plane-only. Both UCMs contain seven alternate `|1` mappings and preserve NTA
reduction notes; the default library encoder does not enable the non-private
fallbacks. Tests check all primary mappings, supplementary and multi-scalar
characters, Roman/kana examples, and representative plane-2 row ordering.

### 15. Attribution and release licensing

The user asked for attribution to mbstring and the project's legal view in the
README. The project position distinguishes observed functional mappings from
copyrightable source code or original compilations, and is explicitly framed
as a position rather than a determination for every jurisdiction. Preserve
the upstream notices and the limits around possible database rights and the
separately built libmbfl probe; do not present the position as legal advice or
as a universal copyright ruling.

For the upcoming release, BSD-3-Clause was selected for the project's own
contributions, matching the upstream jntajis-go license and supporting broad
reuse. `LICENSE` uses “The textcodecs Authors” as the collective holder;
`THIRD_PARTY_NOTICES.md` inventories ICU/Unicode, X0213, ebcdic-kana,
jntajis-go, NTA, and PHP provenance and notices. NTA asks that the JIS
Reduction Map be identified as its work. The license choice does not replace
third-party conditions or provide an express patent grant. Confirm copyright
ownership/contributor rights, third-party notices, and the project's public Go
module path before release.

### Registry-browser work with uncertain placement in this sequence

The current workspace also contains mapping enumeration APIs and a local
codec browser, described in the reference section below. The available
conversation excerpt does not establish where that work occurred relative to
the events above, so it should not be assigned an invented place in the
sequence. Its frontend behavior and testing limitations are recorded below.

## Design and workflow

- Mapping tables are generated ahead of time, embedded with `go:embed`, and read in packed form. They are not materialized in full on first use. Keep representations compact: the wide codec shares decode blocks and BMP bitmap blocks, and uses rank indexes for sparse mappings. Benchmark proposed changes against representative codecs; a compressed `.dat` experiment was dismissed because the size improvement was small.
- `cmd/syncucm` synchronizes the pinned ICU mapping inventory, checks for unexpected additions/removals and GB18030 algorithmic-range changes, regenerates in a staging tree, and validates before applying changes. Its workflow, dependencies, rollback limits, and ICU 74.2 oracle constraint are documented in `docs/icu-data-workflow.md`. The ICU sync does not update the separately maintained X0213, NTA, ebcdic-kana, or PHP data.
- `go generate ./...` rebuilds the embedded `.dat` files and generated registries. Review those outputs with their source mappings and manifests. `go test ./...` is the standard check; `go test -count=1 -tags icu ./...` adds ICU differential tests and needs the documented ICU toolchain. Keep generated files deterministic and test regeneration after changing a generator.
- The wide codec supports Unicode sequences of up to five scalars, including sequences beginning with ASCII. Match the longest available sequence, preserve incomplete prefixes when `atEOF` is false, and emit a decoded sequence atomically. A fast ASCII path must not bypass a possible sequence starting with ASCII.
- Performance is an explicit project goal. ASCII SIMD paths exist for AMD64/ARM64 with a pure-Go fallback. Shared packed lookup tables and direct string indexing are intentional; do not assume a proposed SIMD bitmap search or compressed-table scheme wins without measurement. Avoid encoding-specific special cases in generic SBCS code.

## Mapping identity and compatibility

- Names do not establish mapping identity. Compare both decoding and encoding, including control bytes, private-use areas, alternative/fallback mappings, and preferred encodings. Do not consolidate distinct variants merely because their names or most mappings match. `WalkMappings` enumerates decoding mappings, not all encoder-only alternatives.
- Fifteen PHP SBCS mappings were omitted as exact duplicates of bundled codecs. The eight retained PHP SBCS mappings are distinct; e.g. PHP CP850 and CP866 differ from ICU IBM variants at bytes `1A`, `1C`, and `7F`. See `data/php-mbstring/README.md` and `php_mbstring_test.go`.
- PHP CP936 and `windows-936-2000` have identical decoded mappings but PHP accepts 284 additional Unicode inputs by default. PHP CP950 and Windows-950 differ in encoder preferences, including U+2550. PHP JIS-2004 mappings also differ from Project X0213 in both directions. Preserve these codecs rather than deduplicating by decoding alone; see the comparison tables in `data/php-mbstring/README.md`.
- The library follows ICU default behavior for some unmapped default-ignorable Unicode scalars, consuming them with no output. UCM precision distinctions matter: `|0` round trip, `|3` decode-only, `|4` from-Unicode-only, and non-private `|1` fallbacks generally are not enabled by default. Do not conflate an ignored scalar with an encoder mapping when comparing tables.
- The original ebcdic-kana `ibm290` mapping duplicates ICU `ibm-8482_P100-1999`, not necessarily `ibm-290_P100-1995`. `ibm-290_tilde` and `ibm-290_hitachi-flavored` are distinct. The confusing bare `ibm-290` alias was deliberately removed. Preserve the migration and naming documentation in `data/ebcdic-kana/README.md`.
- Project X0213 Shift_JIS-2004 uses JIS X 0201 Roman: `5C` is U+00A5 and `7E` is U+203E. Its EUC table also maps standalone `8E` and `8F` as C1 controls while using them as prefixes; precedence and streaming behavior are documented in `data/x0213/README.md`. Ordinary EUC-JP must support `8E B1` as U+FF71.

## NTA/JIS data

- `cmd/ntajis` reads the NTA JIS Reduction Map XLSX with an OOXML reader adapted from jntajis-go, produces both UCMs, and compares the source with existing codecs. `make nta-jis-ucm` downloads and verifies the pinned ZIP and XLSX, then regenerates the wide binary and registry. The archive contains `jissyukutaimap1_0_0.xlsx`; the upstream jntajis-go Makefile used a different dependency filename. See `data/nta/README.md` for checksums and commands.
- The spreadsheet has 11,230 source JIS positions, 25 two-scalar mappings, seven alternate Unicode mappings, and 7,940 rows containing reduction information. UCM comments preserve reduction information; encoding conversion does not perform transliteration. Alternate non-private `|1` mappings are not normally enabled by the library encoder.
- `nta-jis-x-0213-euc` covers only the two JIS planes, not ASCII or JIS X 0201 kana. `nta-jis-x-0213-sjis` uses Shift_JIS-2004 byte forms and additionally includes JIS X 0201 Roman and halfwidth kana. Its `5C`/`7E` map to U+00A5/U+203E, and NTA U+FF3C maps to `81 5F`. Plane-2 row order is not numerical; validate changes to the conversion against published tables.
- The generated comparison is in `data/nta/comparison.md` and `nta-jis-comparison.tsv`. ICU EUC-JP and CP932 comparisons intentionally cover only plane 1: their extension areas do not represent JIS X 0213 plane 2.

## PHP extraction and limits

- `cmd/phpmbucm/testdata/probe.c` compiles directly against a separately configured php-src checkout and queries libmbfl conversion functions, including mappings implemented in code. `make php-mbstring-ucm PHP_SRC=/path/to/configured/php-src` regenerates UCMs, source revision and hashes, and binaries. No PHP interpreter is needed to run the probe. Avoid reconstructing PHP behavior from static tables alone.
- PHP outputs represent complete standalone encoded characters, individual Unicode scalars, and decoded multi-scalar sequences. They do not exhaust all possible Unicode sequences or streaming behavior. In particular `php-sjis-mobile-softbank` does not implement its escape-state sequences; stateful ISO-2022/HZ and algorithmic GB18030 are not part of this extraction. See `data/php-mbstring/README.md` before claiming full PHP compatibility.

## Release and attribution

- Project-owned contributions use BSD-3-Clause (`LICENSE`). The adapted jntajis-go reader, ebcdic-kana mappings, ICU/Unicode data, Project X0213 tables, and NTA source have separate notices or terms; see `THIRD_PARTY_NOTICES.md` and the source-specific READMEs. The National Tax Agency requests attribution for its JIS Reduction Map.
- The README states the project's view that individual observed encoding mappings are functional facts and that observation alone does not impose the LGPL on generated data. This is a position, not a universal legal determination; upstream source code retains its notices, database rights can vary by jurisdiction, and distributing a probe binary built with libmbfl raises separate licensing questions. Review these issues and distribution notices before release.
- The module path is still the local placeholder `textcodecs`; a public Go release needs its intended module path settled separately. The configured project copyright notice identifies “The textcodecs Authors” collectively; confirm release ownership and contributor rights before publication.

## Registry inspection and codec browser

- `LookupCodecInfo` reports supported decoded character byte lengths and whether a codec is stateful; for stateful codecs the lengths are the union across states and exclude shift controls. Use this metadata rather than maintaining a second list in the browser. CNS 11643 has three-byte mappings and no two-byte mappings; GB18030 has one-, two-, and four-byte mappings. The one-byte browser view remains useful for navigating prefixes even when those prefixes are not standalone characters.
- `WalkMappings` enumerates decoding mappings in byte order, with a length of one through four bytes, optional byte prefix, half-open big-endian numeric range, decoder context, cancellation, and early stopping. `Context` can carry a shift sequence such as `0E` or an incomplete byte prefix; it is decoded before each candidate and excluded from the reported bytes and text. Do not treat a sequence of two independently decoded single-byte characters as one multibyte mapping. A single mapping can decode to multiple Unicode scalars, so retain the whole output rather than assuming one rune per character.
- In the browser, prefix navigation must follow the streaming decoder: a prefix can be valid at EOF and also need further input when `atEOF` is false (notably X0213 `8E`/`8F`). Shift controls change decoder state rather than representing ordinary glyphs. Keep display behavior based on the library decoder and distinguish unmapped, incomplete, and no-output cases.
- The virtual table covers up to the four-byte space; browsers cannot represent its full pixel height directly. It maps a bounded scroll range to logical rows and offers a hexadecimal jump. The canvas minimap has 16 columns aligned with the table and 256 aggregated rows: a filled cell means at least one mapping exists in that column and row range, not that every byte in the range is mapped. Minimap requests are cancellable and use the public mapping iterator.
- Browser history and shareable URLs retain codec, prefix, width, position, selected cell, and font. Preserve these when changing navigation. The browser is a compact tool; avoid decorative headers or hero areas that reduce space for the table.
- Named Japanese, Korean, and Chinese system fonts are only available if installed. The curated Noto families are loaded from Google Fonts and require network access. Browsers can silently fall back per glyph and do not reliably reveal the actual rendering font; a successful font load is not proof that every character uses it. The browser's bundled Unicode character-name data is Unicode 15.0, so names for newer or unnamed characters may be unavailable.
- `go test ./...` exercises the Go APIs and browser handlers, but it does not validate browser layout, virtual scrolling, history, canvas rendering, or external font loading. Check those in an actual browser when changing the frontend. In restricted environments the default Go build cache may be read-only; `GOCACHE=/tmp/textcodecs-go-cache go test ./...` provides a writable cache.
