gosimilarity
=======

[![Test Status](https://github.com/Songmu/gosimilarity/actions/workflows/test.yaml/badge.svg?branch=main)][actions]
[![Coverage Status](https://codecov.io/gh/Songmu/gosimilarity/branch/main/graph/badge.svg)][codecov]
[![MIT License](https://img.shields.io/github/license/Songmu/gosimilarity)][license]
[![PkgGoDev](https://pkg.go.dev/badge/github.com/Songmu/gosimilarity)][PkgGoDev]

[actions]: https://github.com/Songmu/gosimilarity/actions?workflow=test
[codecov]: https://codecov.io/gh/Songmu/gosimilarity
[license]: https://github.com/Songmu/gosimilarity/blob/main/LICENSE
[PkgGoDev]: https://pkg.go.dev/github.com/Songmu/gosimilarity

Find structurally similar Go functions, methods, and type declarations.
Designed for refactoring discovery in terminals and structured analysis by agents.

## Synopsis

```console
gosimilarity
gosimilarity ./cmd ./internal --cross-file-only
gosimilarity first.go second.go -t 0.9 --print
gosimilarity . --min-tokens 30 --format json
gosimilarity . --exclude 'internal/legacy/**' --include-tests
gosimilarity . --no-functions
gosimilarity . --fail-on-duplicates
```

## Description

gosimilarity parses source without building it, fetching its dependencies, or
performing type checking. Functions and types are both compared by default,
including across files. It scans all collected Go source regardless of build
tags, OS, or architecture constraints.

Function bodies are compared as ordered syntax trees. Parameter, receiver,
result, and local-variable names are normalized using syntactic scopes, including
closures and shadowing. Function names and signatures do not contribute to body
similarity. Type declarations use the same engine, normalizing their own names,
self-references, and type parameters. Only types with the same syntactic RHS kind
are compared; named types are not expanded.

Operators, statement order, field and method names, unresolved identifiers, and
literal source spellings are retained. Comments, positions, and redundant
parentheses are ignored. Ambiguous composite-literal keys are kept by name rather
than guessed to be variable references. A 100% match means **identical normalized
syntax**, not equivalent behavior, interchangeable types, or a recommendation to
automatically merge the code.

```text
Similarity: 100.00%, Score: 3.0 points [function]
  first.go:3-5 Load
  second.go:7-9 Read
  nodes: 18 / 18
```

Results are ranked by `similarity * (linesA + linesB) / 2`, not by name similarity.
Connected components with three or more declarations are displayed as groups,
with every matching pair retained. A group does not imply that every member
matches every other member.

### Options

Paths and options may be interspersed. No paths means `.`; `--` ends options.

| Option | Default | Description |
|---|---|---|
| `--threshold`, `-t` | `0.87` | Similarity in `[0,1]`, not percent |
| `--min-lines`, `-m` | `3` | Minimum physical declaration lines, functions only |
| `--min-tokens` | unset | Minimum normalized tree nodes; overrides `--min-lines` with a warning if both are given |
| `--rename-cost`, `-r` | `0.3` | Finite, positive node replacement cost |
| `--no-functions` | off | Compare only types |
| `--no-types` | off | Compare only functions |
| `--exclude PATTERN` | none | Repeatable cwd-relative glob; `**` matches zero or more path components |
| `--filter-function TEXT` | none | Keep function pairs where either name contains TEXT |
| `--filter-function-body TEXT` | none | Keep function pairs where either original body contains TEXT |
| `--print`, `-p` | off | Include both declarations' original source |
| `--format text\|json` | `text` | Human-readable or structured output |
| `--cross-file-only` | off | Exclude pairs from the same real file |
| `--include-tests` | off | Include automatically discovered `*_test.go` |
| `--fail-on-duplicates` | off | Exit 1 for matches after a complete inspection |
| `--help`, `-h` | | Show usage |
| `--version`, `-V` | | Show version |

Both function filters must hold for a pair, but different sides can satisfy
them. Body searches exclude signatures. Filters and minimum sizes never apply to
types. Disabling both categories is an argument error.

The option names and initial defaults follow
[similarity-ts](https://github.com/mizchi/similarity), but this is an independent
Go implementation, not a port or score-compatible replacement. Unsupported
options (including size penalties, class analysis, and ignore comments) are not
silently accepted.

### Discovery

Directories are scanned recursively. Within a Git working tree, Git must be
available: `git ls-files --cached --others --exclude-standard` selects tracked and
non-ignored untracked Go files. Git's standard ignore rules apply, including
global ignores and `.git/info/exclude`. Tracked ignored files remain eligible;
the current working-tree contents, not the index contents, are parsed. Missing
tracked files are reported as normal skips. A Git failure is an error, never a
fallback to a different file set. Outside Git, ordinary recursive walking is
used without Git ignore rules.

Automatic discovery skips `.git`, `vendor`, generated Go source, tests, and all
symlinks. `--include-tests` lifts only the test exclusion. Explicit file paths
override automatic exclusions and Git ignores. Explicit directory paths do not.
Only regular files are parsed: discovered special files such as FIFOs are
skipped, while explicitly naming a non-regular file is an error.
Explicit symlinks resolve to their real file or directory; subsequently
discovered symlinks are still skipped. Real paths deduplicate inputs and identify
files for `--cross-file-only`, independently of argument order.

Explicit `--exclude` always wins, even over direct file inputs. Patterns match
slash-separated real paths relative to the real cwd, not relative to each input
root. `*`, `?`, and character classes use Go's `path.Match` syntax; a whole `**`
component matches any depth. Matching a directory also excludes its descendants.
Patterns are case-sensitive and use `/` on every platform.

### Distance and limits

The ordered-subtree edit distance uses zero cost for equal labels, configurable
replacement cost (default `0.3`), and the subtree node count for insertion or
deletion. Children are aligned by dynamic programming; deleting a node does not
promote its children. Replacing an entire tree by deletion and insertion charges
**both** operations. Similarity is `max(0, 1 - distance/max(nodesA,nodesB))`.
Role wrappers and empty slots count as nodes, so `--min-tokens` is not a lexical
token count. Signatures can establish bindings without contributing tree nodes.

Interned subtrees are verified structurally, and size differences provide a safe
distance lower bound. Only actual equality can produce similarity 1 or a text
display of `100.00%`; non-exact text output is capped at `99.99%` without changing
the underlying score or JSON value.

All eligible pairs and all matches are retained: there is no hidden top-K limit
or approximate search. Pair enumeration is quadratic. Large, mostly identical
inputs can require substantial time and memory, especially for full JSON output.
For example, 10,000 identical functions alone produce 49,995,000 pairs.
Use size limits, file exclusions, function filters, or exact-match thresholds
when appropriate; one-line types are always eligible.

### JSON and exit status

Reports go to stdout; logs use `slog` on stderr. `--format json` writes exactly
one JSON document to stdout, never mixed with logs. Warnings and errors also
appear on stderr. Skip logs are debug-level and suppressed by default; their
diagnostics remain in the structured report.
`--print` adds `source` fields.

The JSON document has these fields:

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
`end.column` and `end.offset` are exclusive. Types use the individual TypeSpec
range (the name through its RHS), including inside grouped or local declarations.
`type_kind` uses the Go AST RHS node name with the `Type` suffix removed, except
arrays and slices, which are `"array"` and `"slice"`. Parentheses do not affect
classification.

| Exit | Meaning |
|---|---|
| `0` | Complete inspection, regardless of matches by default |
| `1` | Complete inspection with matches and `--fail-on-duplicates` |
| `2` | Invalid arguments or an incomplete inspection |

Read and parse failures exclude the affected file; normalization failures exclude
only the affected declaration. Other comparisons continue and partial results are
preserved with `"incomplete"` status. Errors take precedence over duplicate
failure. Normal skips do not make an inspection incomplete.

## Installation

```console
# Install the latest version. (Install it into ./bin/ by default).
% curl -sfL https://raw.githubusercontent.com/Songmu/gosimilarity/main/install.sh | sh -s

# Specify installation directory ($(go env GOPATH)/bin/) and version.
% curl -sfL https://raw.githubusercontent.com/Songmu/gosimilarity/main/install.sh | sh -s -- -b $(go env GOPATH)/bin [vX.Y.Z]

# In alpine linux (as it does not come with curl by default)
% wget -O - -q https://raw.githubusercontent.com/Songmu/gosimilarity/main/install.sh | sh -s [vX.Y.Z]

# go install
% go install github.com/Songmu/gosimilarity/cmd/gosimilarity@latest
```

## Development

```console
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchtime=1x -benchmem
```

Benchmarks use generated, synthetic source: 10,000 functions plus 100 types with
exact matching and with a one-function filter, and a 10,001-node wide tree pair.
They measure different workloads, not a promise of default-threshold performance
on arbitrary repositories.

After changing GitHub Actions dependencies, run `gh actions-lock` and commit
`.github/workflows/actions.lock` with the workflow changes. Install the tool
with `gh extension install github/gh-actions-lock`.

## Author

[Songmu](https://github.com/Songmu)
