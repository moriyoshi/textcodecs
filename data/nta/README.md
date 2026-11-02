# NTA JIS mapping data

The [National Tax Agency JIS Reduction Map version 1.0.0](https://www.houjin-bangou.nta.go.jp/download/) is a National Tax Agency work. Attribution to the National Tax Agency is required when using it; see the agency's [terms of use](https://www.houjin-bangou.nta.go.jp/riyokiyaku/index.html). The generated UCM and comparison are independently produced from that map, not official agency publications.

The XLSX reader is adapted from [jntajis-go](https://github.com/kenall-inc/jntajis-go); its license is in [JNTAJIS-LICENSE](JNTAJIS-LICENSE).

Run `make nta-jis-ucm` from the repository root. Like the upstream Makefile, this downloads and extracts the agency's ZIP and generates the output. It verifies SHA-256 hashes for the ZIP and workbook, caches them under `.cache/nta-jis/`, and regenerates the bundled multibyte tables. Override `NTA_CACHE` to use another directory. To use an existing workbook directly, run `go run ./cmd/ntajis -input /path/to/jissyukutaimap1_0_0.xlsx`; use `-sha256` to require a specific workbook hash, then run `go generate ./...` to update the bundled tables.

The generator produces:

* [nta-jis-x-0213-euc.ucm](nta-jis-x-0213-euc.ucm), JIS X 0213 positions represented in EUC-JIS-2004 byte form, registered as `nta-jis-x-0213-euc`. It covers the spreadsheet's two planes only, not ASCII or JIS X 0201 kana.
* [nta-jis-x-0213-sjis.ucm](nta-jis-x-0213-sjis.ucm), registered as `nta-jis-x-0213-sjis`, represents the same JIS planes in Shift_JIS-2004 form and adds JIS X 0201 Roman and halfwidth kana. The Roman bytes `5C` and `7E` decode to U+00A5 and U+203E, respectively; U+FF3C is represented by the NTA two-byte mapping `81 5F`.
* [comparison.md](comparison.md), a summary against the existing Japanese codecs, and [nta-jis-comparison.tsv](nta-jis-comparison.tsv), the individual differences. Plane 2 is excluded for ICU EUC-JP and CP932 because their extension areas have different meanings.

In both mappings the seven alternate Unicode values are UCM encoder fallback mappings; like other non-private ICU fallbacks, they are not enabled by the library's default encoder. Reduction information is preserved in comments, not applied as transliteration.

The workbook SHA-256 is `61973084e775593964d2bc3358c223bb01481f1b6555634a420682a2d649405b`. The output can be checked with ICU's `makeconv -d /tmp data/nta/nta-jis-x-0213-*.ucm`.
