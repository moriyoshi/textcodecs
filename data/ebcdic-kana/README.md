# EBCDIC-kana mappings

These mappings originated in the `charmap` directory of
[github.com/moriyoshi/ebcdic-kana](https://github.com/moriyoshi/ebcdic-kana)
at the commit recorded in [UPSTREAM_REVISION](UPSTREAM_REVISION). The source
repository's [license](LICENSE) is retained here, and ICU-derived data remains
subject to [ICU-LICENSE](../../ICU-LICENSE).

The nine UCM files retain the upstream mapping contents, with file names and
`<code_set_name>` headers changed for this library. Their current hashes are
listed in [ucm.sha256](ucm.sha256). The shared single-byte generator consumes
them alongside the ICU UCMs. They are maintained here and are not updated by
the ICU sync command.

The upstream `ibm290` mapping is identical in both directions to the bundled
ICU `ibm-8482_P100-1999`, so it is not bundled separately. Its original UCM
SHA-256 was `b5fb34ea98e01589b9cc2119ce1f68f31ab07d1bc3ba84c1bad70065bab6058d`.
ICU's separate `ibm-290_P100-1995` does not map byte `E1` to U+20AC, whereas
`ibm-8482_P100-1999` does. `ibm-290_tilde` leaves `E1` and `A0` unmapped and
maps `A1` to U+007E.

The names exposed by `Open` and `LookupSingleByte` are:

- `ebcdic-nec` and `ebcdic-nec_lf-nl-alt`
- `ebcdik` and `ebcdik_lf-nl-alt`
- `ibm-290_hitachi-flavored` and `ibm-290_tilde`
- `ibm-298`, `ibm-298_backslash`, and `ibm-298_hitachi-flavored`

The original package and this library have different APIs and error behavior.
The original decoder emits U+FFFD for undefined bytes; this library reports
`ErrUnmapped` and leaves the byte unconsumed by default. `OpenWithOptions`
with `DecodeMode: ErrorReplace` emits U+FFFD instead. This library also consumes
some unmapped default-ignorable Unicode scalars when encoding, following its ICU
compatibility behavior. Callers migrating from the original package should
handle these differences explicitly.
