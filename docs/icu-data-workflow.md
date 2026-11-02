# ICU mapping data workflow

This document describes the current update process, its outputs, and the
checks that can require manual work. The upstream inputs are the UCM files in
ICU's `icu4c/source/data/mappings` tree. The selected ICU commit is recorded
in `data/icu/UPSTREAM_REVISION`; `data/icu/ucm-inventory.sha256` records the
SHA-256 of each bundled UCM by filename. The inventory currently contains 190
files, all exposed by the library.

## Prerequisites

Run the commands from the repository root. Updating requires Git, a Go
toolchain compatible with `go.mod`, ICU **74.2** development files and tools
(including `makeconv`), `pkg-config`, and a C compiler. The differential-test
oracle explicitly rejects other ICU library versions. The selected upstream
source revision can be newer than 74.2, but the tests still use the installed
74.2 runtime and its behavior; a source update that needs a newer ICU oracle
requires a separate test and compatibility change. Normal library builds do
not require ICU or Git.

## Updating from ICU

1. Choose a source revision. A commit ID or release tag makes the update
   reproducible. For example, with an existing ICU checkout:

   ```sh
   go run ./cmd/syncucm -repo /path/to/icu -ref <commit-or-tag> -check
   ```

   Without `-repo`, the command uses `https://github.com/unicode-org/icu.git`;
   without `-ref`, it uses `main`. Remote sources are fetched into a temporary
   Git repository. A local `-repo` must already contain the selected commit and
   required objects; uncommitted edits in that checkout are ignored. A partial
   checkout may need network access to obtain missing Git objects. The
   `-root` option selects a different textcodecs workspace.

2. The check performs the complete staging and validation described below. A
   built `syncucm` command exits 0 if already synchronized, 1 if validated
   updates are available in check mode, and 2 on an error. `go run` reports a
   nonzero program exit through the Go tool, so build the command if an
   automation needs the exact exit status. A check can fail before reporting
   differences if the revision requires manual changes.

3. Once validation succeeds, run the same command without `-check` to apply
   the result. This performs the fetch and validation again; use an immutable
   commit ID to ensure both invocations select the same source.

4. Review the resulting UCM, manifest, binary and generated Go changes together
   with the revision. Inspect upstream release notes and license changes as
   appropriate; `ICU-LICENSE` is maintained separately and is not synced by the
   command. Commit the resulting files together after review.

## What the sync command does

1. Read the inventory and check that each local UCM exists exactly once and
   matches its recorded SHA-256. Snapshot the UCMs, generated artifacts,
   inventory, and revision so that local changes are not silently overwritten.
2. Resolve the selected ICU ref to a commit and read the committed mapping
   blobs and `icu4c/source/common/ucnvmbcs.cpp` via Git. Require the upstream
   UCM filenames to match the local inventory exactly. Additions and removals
   stop the sync for implementation and inventory review.
3. Compare the GB18030 algorithmic range rows in the upstream C++ source with
   `gb18030.go`, including their linear endpoints. This checks the ranges, not
   all upstream GB18030 algorithm code; review other relevant upstream changes
   separately.
4. Copy the workspace to a temporary directory and run `go generate ./...` on
   the existing inputs. Compare every expected output with the snapshot. If
   an output is stale or locally modified, stop and reconcile it first.
5. Replace the staged UCMs with the upstream blobs, write a sorted inventory and
   the resolved commit ID, and run `go generate ./...` again. Run
   `go test -count=1 -tags icu ./...` in the temporary workspace. This includes
   the normal Go tests and ICU differential tests; the latter compile UCMs
   with `makeconv` and compare conversion behavior using the ICU oracle.
6. In check mode, report whether any target would change and leave the
   workspace untouched. Otherwise, check that targets still match their
   original snapshots, then replace changed files individually using temporary
   files and renames. On a write failure, the command attempts to roll back
   files already written. The group of files is not a single atomic transaction;
   inspect any reported rollback error before continuing.

## Generated artifacts

`go generate ./...` invokes the directives in the codec source files. Each
binary generator emits a binary, a Go registry with offsets and properties,
consistent with that binary, and a source-hash manifest:

| UCM input | Generator | Binary | Go metadata | Hash manifest |
| --- | --- | --- | --- | --- |
| `data/icu/mappings/*.ucm` and `data/ebcdic-kana/*.ucm` | `cmd/gensbcs` | `data/icu/sbcs.dat` | `sbcs_generated.go` | `data/icu/sbcs.sha256` (ICU sources) |
| `data/icu/mbcs2/*.ucm` and `data/icu/ebcdic/*.ucm` | `cmd/genmbcs2` | `data/icu/mbcs2.dat` | `mbcs2_generated.go` | `data/icu/mbcs2.sha256` |
| `data/icu/mbcswide/*.ucm`, `data/x0213/*-std.txt`, `data/nta/nta-jis-x-0213-*.ucm` and selected PHP mappings | `cmd/genmbcswide` | `data/icu/mbcswide.dat` | `mbcswide_generated.go` | `data/icu/mbcswide.sha256` (ICU sources) |
| `data/icu/ebcdicwide/*.ucm` | `cmd/genebcdicwide` | `data/icu/ebcdicwide.dat` | `ebcdicwide_generated.go` | `data/icu/ebcdicwide.sha256` |

`cmd/gengsm` reads `data/icu/gsm-03.38-2009.ucm` and produces
`gsm_generated.go` directly, with the source hash in the generated file; it
has no `.dat` file or separate generator manifest. All UCM files, including
GSM, are also covered by the overall inventory. The individual `cmd/ucmgen`
command generates a standalone single-byte codec from an explicitly supplied
UCM; it is not part of the bundled-data sync pipeline.

The wide generator also consumes the separately maintained
[Project X0213 source tables](../data/x0213/README.md). Their checksums are
verified by tests; they are not ICU UCMs and are not included in the ICU
manifests or updated by the ICU sync command.

The single-byte generator also consumes the separately maintained
[ebcdic-kana UCMs](../data/ebcdic-kana/README.md). Their hashes are recorded
separately and they are not updated by the ICU sync command. The upstream
`ibm290` table is omitted because it duplicates ICU's `ibm-8482_P100-1999`.

Generators parse the UCM state tables and mappings and construct their compact
lookup representations ahead of time. For example, the two-byte generator
shares identical decode blocks and stores sparse encode blocks using masks;
the wide generator uses the same shared 32-entry block indexing for short
encoded sequences, with three-byte Unicode values for supplementary scalars.
For BMP encoding it also shares 64-scalar bitmap blocks, addressed by direct
offsets; supplementary encoding groups use bitmaps with precomputed
word-prefix counts. Binary
multi-byte values are written in little-endian form. The generated Go files contain the registry
and offsets that identify the data for each encoding. The library embeds the
`.dat` files with `go:embed` and reads their packed data directly at runtime;
it does not download UCM files or expand all mappings on first use. GSM tables
are compiled from its generated Go source instead.

## Manual regeneration and exceptional updates

To regenerate the artifacts from the *currently bundled* UCM files without
fetching ICU, run:

```sh
go generate ./...
go test ./...
go test -count=1 -tags icu ./...
```

`go generate` writes generated files in place. It does not update the overall
inventory or the upstream revision and is not a substitute for `syncucm`
when importing a new ICU commit. Review its output with the repository's
version-control tools. Never update a checksum or a generator's vetted hash
just to bypass a failed check.

Some UCMs contain inherited mappings or special state tables for which the
generators accept only explicitly vetted hashes; GB18030 and GSM also have
pinned inputs. A change to one of these inputs fails generation until its
semantics and supporting implementation are reviewed. Likewise, a changed
file inventory, changed GB18030 ranges, unsupported UCM features, an ICU
version mismatch, or differential-test failures require investigation and
possibly code or test changes before rerunning the sync. The command does not
merge inherited mapping bases or automatically adapt to new converter formats.
