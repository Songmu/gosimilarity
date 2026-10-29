# CLI reference

```text
gosimilarity [options] [files or directories...]
gosimilarity --packages [options] [package patterns...]
```

Paths and options may be interspersed. No paths means `.`; `--` ends options.

## Options

| Option | Default | Description |
|---|---|---|
| `--threshold`, `-t` | `0.87` | Minimum similarity in `[0,1]`, not percent |
| `--min-lines`, `-m` | `3` | Minimum physical declaration lines, functions only |
| `--min-tokens` | unset | Minimum normalized syntax nodes, not lexical tokens; overrides `--min-lines` |
| `--rename-cost`, `-r` | `0.3` | Finite, positive syntax-node replacement cost |
| `--types` | off | Also compare type declarations |
| `--no-functions` | off | Compare only types |
| `--no-types` | off | Compare only functions |
| `--exclude PATTERN` | none | Repeatable cwd-relative glob; `**` matches zero or more path components |
| `--filter-function TEXT` | none | Keep function pairs where either name contains TEXT |
| `--filter-function-body TEXT` | none | Keep function pairs where either original body contains TEXT |
| `--print`, `-p` | off | Include both declarations' original source |
| `--format text\|json` | `text` | Human-readable or structured output |
| `--cross-file-only` | off | Exclude pairs from the same real file |
| `--include-tests` | off | Include automatically discovered `*_test.go` |
| `--packages` | off | Select build-eligible files using Go package patterns |
| `--tags TAGS` | unset | Additional comma-separated build tags; requires `--packages` |
| `--fail-on-duplicates` | off | Exit 1 for matches after a complete inspection |
| `--help`, `-h` | | Show usage |
| `--version`, `-V` | | Show version |

Minimum sizes must be nonnegative. Supplying both `--min-tokens` and
`--min-lines` explicitly produces a warning; `--min-tokens` takes precedence.
Filters and minimum sizes never apply to types.

Both function filters must hold for a pair, but different sides can satisfy
them. Body searches exclude signatures. Disabling both categories, or combining
`--types` with `--no-types`, is an argument error.

## File selection

Directories are scanned recursively. Within a Git working tree, Git must be
available. Tracked and non-ignored untracked Go files are eligible, including
tracked files that match an ignore rule. Git's standard ignore rules apply,
including global ignores and `.git/info/exclude`. The current working-tree
contents are inspected, not the index contents. A Git failure is an error,
not a fallback to a different file set.

Outside Git, directories are scanned without applying Git ignore rules.
Automatic discovery skips `.git`, `vendor`, generated Go source, tests, and
symlinks. `--include-tests` lifts only the test exclusion. Explicit file paths
override automatic exclusions and Git ignores; explicit directory paths do not.
Build tags, OS, and architecture constraints do not affect this default file
selection mode.

Only regular files are parsed. Discovered special files such as FIFOs and
missing tracked files are normal skips; explicitly naming a non-regular file
is an error. Explicit symlinks resolve to their real file or directory;
subsequently discovered symlinks are still skipped. Real paths deduplicate
inputs and identify files for `--cross-file-only`.

### Package selection

`--packages` uses `go list -e -find -json` instead of directory or Git discovery.
Arguments are Go package patterns, not individual `.go` files. No arguments
means `.` (the current package only); use `./...` to include packages
recursively. Multiple and overlapping patterns are supported, and files are
deduplicated before comparison. Comparisons can span selected packages.

```sh
gosimilarity --packages ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 gosimilarity --packages ./...
gosimilarity --packages --tags=integration,netgo --include-tests ./...
```

The Go command selects source files using its build environment, including
`GOOS`, `GOARCH`, `CGO_ENABLED`, and build constraints. The target need not
match the host running gosimilarity. `GOFLAGS` is inherited; an explicit
`--tags` overrides its `-tags` setting, including `--tags=` to clear it.
Go must be available on `PATH`, and package patterns must be resolvable in the
current Go module, workspace, or GOPATH context.

Selection includes original `GoFiles` and `CgoFiles`; `--include-tests` adds
build-eligible `TestGoFiles` and `XTestGoFiles`, not test binaries or generated
cgo compiler input. Dependencies are not resolved or automatically added,
and packages are not compiled or type-checked.

Git ignores do not apply in this mode. Generated source, symlinks, and
automatically excluded paths are still skipped, and `--exclude` still wins.
Excluding a package directory also suppresses its package-loading errors;
excluding individual files does not change how Go loads the package.
File-loading and `go list` errors make the inspection incomplete (exit 2);
available partial results are preserved, with no fallback to all-target
discovery. Go command warnings are included in diagnostics.

### Exclusions

Explicit `--exclude` always wins, even over direct file inputs. Patterns match
slash-separated real paths relative to the real current working directory,
not relative to each input root.

```sh
gosimilarity . --exclude 'internal/legacy/**' --exclude '**/fixtures/**'
```

`*`, `?`, and character classes use Go's `path.Match` syntax; a whole `**`
component matches any depth. Matching a directory also excludes its descendants.
Patterns are case-sensitive and use `/` on every platform.

## Matching and ranking

Functions and methods are compared by their bodies. Function names and
signatures do not contribute to similarity. Parameter, receiver, result, and
local-variable names are normalized, respecting scopes, closures, and shadowing.
Comments, positions, and redundant parentheses are ignored.

Operators, statement order, field and method names, unresolved identifiers,
and literal source spellings remain significant. Ambiguous composite-literal
keys are kept by name rather than guessed to be variable references.

Type comparisons retain declared names, self-references, field names, field
types, and tags while normalizing type parameter names. Only types with the
same syntactic kind are compared; named types are not expanded.

Results are ranked by `similarity * (linesA + linesB) / 2`, so larger matches
can rank ahead of smaller matches with higher similarity. Groups contain
three or more declarations connected by matching pairs. A group does not
imply that every member matches every other member.

All eligible pairs are considered and all matches are reported; there is no
hidden result limit. A text display of `100.00%` is reserved for identical
normalized syntax. JSON similarity values are not rounded.

## JSON output

`--format json` writes exactly one JSON document to stdout. Warnings and
errors go to stderr and also appear in the report's diagnostics. Normal skip
messages are not printed by default, but remain in the report.
`--print` adds declaration `source` fields.

| Field | Contents |
|---|---|
| `status` | `"complete"` or `"incomplete"` |
| `summary` | File, skip-reason, filtered-function, declaration, eligible-candidate, pruned-pair, distance-call, and match counts; function/type candidate counts are separate |
| `declarations` | `id`, `kind` (`function` or `type`), `name`, `file`, `start`, `end`, `nodes`; function `signature` and optional `receiver`; type `type_kind` and optional true `alias` |
| `pairs` | `id`, `a`, `b` (declaration IDs), unrounded `similarity`, and ranking `score` |
| `groups` | `id`, `kind`, `members` (declaration IDs), `pairs` (all matching pair IDs); only components of at least three members |
| `diagnostics` | `severity` (`info`, `warning`, `error`), `stage`, `message`, optional `file` and `declaration` |

Declaration IDs (`d1`, ...) follow canonical-file and source order. Pair IDs
(`p1-2`, ...) use declaration indices. Group IDs (`g1`, ...) follow ranked
components. IDs resolve within the document and are deterministic for the same
inputs and options, not persistent identities across source edits. Pairs are
ranked by descending score, with file and source order breaking ties.

Positions use physical source locations, ignoring `//line`: one-based lines and
byte columns, zero-based byte offsets. `end.line` is the last occupied line;
`end.column` and `end.offset` are exclusive. Type ranges cover the individual
declared name through its right-hand side, including inside grouped or local
declarations. `type_kind` uses the Go AST right-hand-side node name with the
`Type` suffix removed, except arrays and slices, which are `"array"` and
`"slice"`. Parentheses do not affect classification.

## Exit status and errors

| Exit | Meaning |
|---|---|
| `0` | Complete inspection, regardless of matches by default |
| `1` | Complete inspection with matches and `--fail-on-duplicates` |
| `2` | Invalid arguments or an incomplete inspection |

Read and parse failures exclude the affected file; normalization failures
exclude only the affected declaration. Other comparisons continue and partial
results are preserved with `"incomplete"` status. Errors take precedence over
duplicate failure. Normal skips do not make an inspection incomplete.

Return to the [README](../README.md) for installation and common examples.
