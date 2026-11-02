# National Tax Agency JIS mapping comparison

Source: [National Tax Agency JIS Reduction Map, version 1.0.0](https://www.houjin-bangou.nta.go.jp/download/); spreadsheet SHA-256 `61973084e775593964d2bc3358c223bb01481f1b6555634a420682a2d649405b`. This is an independently generated analysis of a National Tax Agency work.

The spreadsheet contains **11230 JIS positions**, including 25 two-scalar Unicode mappings, 7 alternate Unicode mappings, and 7940 rows with reduction information. Reduction values describe transliteration and are retained as UCM comments; they are not applied by these codec comparisons.

The generated UCM uses EUC-JIS-2004 byte forms for the spreadsheet's JIS positions only; it does not include ASCII or JIS X 0201 kana. Decoding compares each JIS position; encoding compares each primary Unicode sequence against that position. ‘Missing’ includes invalid or unmapped inputs. ‘Existing only’ counts positions mapped by the existing codec but absent from the spreadsheet. Differences are listed in [nta-jis-comparison.tsv](nta-jis-comparison.tsv).

| Existing codec | Scope | Decode same | Decode different | NTA-only | Existing only | Encode same | Encode different | Encode missing |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `euc-jis-2004` | planes 1 and 2 | 11221 | 9 | 0 | 3 | 11221 | 3 | 6 |
| `shift_jis-2004` | planes 1 and 2 | 11222 | 8 | 0 | 3 | 11222 | 2 | 6 |
| `euc-jp-2007` | plane 1 only | 6952 | 374 | 1468 | 10 | 6952 | 1252 | 590 |
| `ibm-943_P15A-2003` | plane 1 only | 6952 | 374 | 1468 | 10 | 6952 | 200 | 1642 |

Alternate Unicode values, compared with default encoder behavior:

| Existing codec | Same | Different | Missing |
| --- | ---: | ---: | ---: |
| `euc-jis-2004` | 7 | 0 | 0 |
| `shift_jis-2004` | 7 | 0 | 0 |
| `euc-jp-2007` | 0 | 0 | 7 |
| `ibm-943_P15A-2003` | 0 | 0 | 7 |

The `encode-alternate` rows in the TSV describe differences for those values, which are fallback mappings in the UCM when unambiguous. The ICU EUC-JP and CP932 comparisons are limited to plane 1 because their extension areas do not represent JIS X 0213 plane 2. `shift_jis-2004` uses the allocated plane-2 rows of Shift_JIS-2004. Comparisons use the checked-in codec tables and their strict, default fallback behavior.
