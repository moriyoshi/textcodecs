# PHP mbstring mappings

These UCMs are generated from conversion functions and their tables in PHP's
`ext/mbstring/libmbfl` at the revision recorded in [UPSTREAM_REVISION](UPSTREAM_REVISION).
They are named `php-<mbstring name>` so that PHP-specific behavior is not
confused with similarly named ICU or Project X0213 mappings. The source files
carry their own copyright and license notices; consult the PHP checkout for
those notices and the provenance of its individual tables.

The files cover the 23 single-byte encodings in `mbfilter_singlebyte.c` and
19 stateless or mostly stateless CJK variants in `mbfilter_cjk.c`. The three
mobile Shift_JIS filenames use hyphens instead of `#` in PHP's names. The 27
distinct mappings listed in [registry-sbcs.txt](registry-sbcs.txt) and
[registry-wide.txt](registry-wide.txt) are bundled under their `php-` names.
The remaining 15 have identical decoding and encoding mappings to already
registered codecs and are not registered again:

| PHP UCM | Existing codec |
| --- | --- |
| `php-iso-8859-2` through `php-iso-8859-9` | `ibm-912_P100-1995`, `ibm-913_P100-2000`, `ibm-914_P100-1995`, `ibm-915_P100-1995`, `ibm-1089_P100-1995`, `ibm-9005_X110-2007`, `ibm-5012_P100-1999`, `ibm-920_P100-1995`, respectively |
| `php-iso-8859-10`, `-13`, `-14`, `-15` | `iso-8859_10-1998`, `ibm-921_P100-1995`, `iso-8859_14-1998`, `ibm-923_P100-1998`, respectively |
| `php-koi8-r`, `php-koi8-u`, `php-windows-1252` | `ibm-878_P100-1996`, `ibm-1168_P100-2002`, `ibm-5348_P100-1997`, respectively |

The eight PHP single-byte mappings that remain in the registry differ from all
other registered single-byte codecs in their mapping tables. In particular:

| PHP codec | Comparison with an existing ICU codec |
| --- | --- |
| `php-ascii` | Rejects bytes above `7F`; `ibm-1089_P100-1995` accepts 83 additional byte values. |
| `php-armscii-8` | Provides Armenian mappings; no other registered single-byte codec has the same table. |
| `php-cp850`, `php-cp866` | Each differs from its corresponding IBM codec at control bytes `1A`, `1C`, and `7F`, in both directions. |
| `php-iso-8859-1` | Differs from `ibm-922_P100-1999` at four byte positions. |
| `php-iso-8859-16` | Differs from `ibm-923_P100-1998` at 32 byte positions. |
| `php-windows-1251` | Rejects `98`; `ibm-5347_P100-1998` maps it to U+0098. |
| `php-windows-1254` | Rejects seven byte values that `ibm-5350_P100-1998` maps to C1 controls. |

These comparisons cover all 256 byte values and all BMP Unicode scalars in
both directions. The library's silently ignored Unicode characters are not
counted as mappings.

The JIS-2004 mappings are retained because they differ from Project X0213.
For example, PHP Shift_JIS-2004 decodes `81 5F` and `81 B0` as U+FF3C and
U+FF5E; Project X0213 decodes them as U+005C and U+007E. PHP EUC-JP-2004
decodes `A1 B1` and `A1 EF` as U+FFE3 and U+FFE5; Project X0213 uses U+203E
and U+00A5. The encoders also make different choices for these characters.

The other PHP mappings were compared with related ICU codecs, using the
library's default mapping behavior in both directions. Each remains distinct:

| PHP codec | ICU codec compared | Decode differences | Encode differences |
| --- | --- | --- | --- |
| `php-cp850` | `ibm-850_P100-1995` | 3 different | 3 different |
| `php-cp866` | `ibm-866_P100-1995` | 3 different | 3 different |
| `php-big5` | `windows-950-2000` | 259 different; ICU has 5,978 more | 6 different; PHP has 257 more, ICU 6,233 more |
| `php-cp950` | `windows-950-2000` | ICU has 3 more | 4 different; ICU has 3 more |
| `php-cp936` | `windows-936-2000` | Identical | PHP accepts 284 more |
| `php-euc-cn` | `ibm-1383_P110-1999` | 1 different; ICU has 1,421 more | 1 different; ICU has 1,421 more |
| `php-euc-kr` | `ibm-970_P110_P110-2006_U2` | PHP has 2 more; ICU has 218 more | PHP has 2 more; ICU has 218 more |
| `php-euc-tw` | `euc-tw-2014` | 13 different; PHP has 10,061 more, ICU 48,740 more | 4,204 different; PHP has 6 more, ICU 44,543 more |

Counts compare complete single encoded characters and individual Unicode
scalars; encode counts do not treat silently ignored characters as mappings.
For EUC-TW the four-byte comparison covers `8E A1`–`8E B0` plane prefixes and
`A1`–`FE` row and cell bytes. The counts include control and private-use
characters. For example, PHP CP936 accepts U+203E as an encoder-only mapping,
whereas the ICU codec does not enable that non-private fallback by default.

For every complete encoded character, the extractor asks mbstring's decoder
for the Unicode sequence. It asks the encoder separately about every Unicode
scalar and each sequence produced by the decoder. Round trips use `|0`,
decode-only mappings use `|3`, and encode-only mappings use `|4`. This includes
PHP's preferred byte sequence for duplicate mappings and its encoder-only
alternatives. The substitution byte in each UCM is `3F`; error replacement
behavior is outside this mapping extraction.

This is not a complete description of streaming behavior. In particular,
SoftBank's mobile Shift_JIS decoder uses escape sequences, and those state
transitions are not captured by `php-sjis-mobile-softbank`; it provides only
the standalone-character mapping. Encoders could
accept Unicode sequences that are not produced by decoding a single character;
the probe does not enumerate every possible sequence. Stateful ISO-2022/HZ,
UTF encodings, transfer encodings and algorithmic GB18030 variants are not
included. The latter can have over a million mappings, so emitting an explicit
UCM would be substantially larger than an algorithmic representation.

To regenerate, configure a copy of the PHP source (a complete PHP build is not
required), then run:

```sh
make php-mbstring-ucm PHP_SRC=/path/to/configured/php-src
makeconv -d /tmp data/php-mbstring/*.ucm
```

For example, a clean copy can be prepared with `./buildconf --force` followed by
`./configure --disable-all --enable-cli --enable-mbstring --disable-mbregex`.
The target compiles a small C probe against that checkout; it does not run PHP.
It records the checkout's Git revision, or accepts an explicit `PHP_REVISION`
for a source tree without Git metadata. Ensure the checkout is clean so that
the recorded revision describes the inputs. The target also refreshes
[ucm.sha256](ucm.sha256) and regenerates the bundled binary tables. The compiler command uses GNU-compatible section
garbage collection and is intended for a Unix build environment.
