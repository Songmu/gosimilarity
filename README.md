# gosimilarity

[![Test Status](https://github.com/Songmu/gosimilarity/actions/workflows/test.yaml/badge.svg?branch=main)][actions]
[![Coverage Status](https://codecov.io/gh/Songmu/gosimilarity/branch/main/graph/badge.svg)][codecov]
[![MIT License](https://img.shields.io/github/license/Songmu/gosimilarity)][license]
[![PkgGoDev](https://pkg.go.dev/badge/github.com/Songmu/gosimilarity)][PkgGoDev]

[actions]: https://github.com/Songmu/gosimilarity/actions?workflow=test
[codecov]: https://codecov.io/gh/Songmu/gosimilarity
[license]: LICENSE
[PkgGoDev]: https://pkg.go.dev/github.com/Songmu/gosimilarity

gosimilarity is a Go code similarity tool inspired by
[mizchi/similarity](https://github.com/mizchi/similarity). It finds structurally
similar functions, methods, and type declarations to help identify refactoring
opportunities, even when local variable names differ.

Use readable terminal reports to explore matches, or JSON output for scripts
and agents.

## Installation

Install with Go using the version required by [go.mod](go.mod) or later:

```sh
go install github.com/Songmu/gosimilarity/cmd/gosimilarity@latest
```

Or download a prebuilt binary from [GitHub Releases][releases]. The installer
selects the appropriate binary for your platform and installs it into `./bin`
by default:

```sh
curl -sfL https://raw.githubusercontent.com/Songmu/gosimilarity/main/install.sh | sh -s
./bin/gosimilarity --version
```

To choose an installation directory, pass `-b`; append a release tag to select
a specific version:

```sh
curl -sfL https://raw.githubusercontent.com/Songmu/gosimilarity/main/install.sh | sh -s -- -b "$HOME/.local/bin"
```

Make sure your installation directory is on `PATH` when using the commands below.

[releases]: https://github.com/Songmu/gosimilarity/releases

## Quick start

Scan the current directory for similar functions and methods:

```sh
gosimilarity .
```

Matches include their similarity, ranking score, and source locations:

```text
Similarity: 100.00%, Score: 3.0 points [function]
  first.go:3-5 Load
  second.go:3-5 Read
  nodes: 5 / 5
```

The default similarity threshold is `0.87` (87%), and functions must span at
least three lines. Narrow the search or inspect the matching source:

```sh
gosimilarity ./cmd ./internal --cross-file-only
gosimilarity . --threshold 0.95 --min-lines 10 --print
gosimilarity . --exclude 'internal/legacy/**' --include-tests
```

Type declarations are opt-in:

```sh
gosimilarity . --types         # Functions, methods, and types
gosimilarity . --no-functions  # Types only
```

## Scripts and CI

Write a single JSON report to stdout; diagnostics go to stderr:

```sh
gosimilarity . --format json > similarity.json
```

To fail CI when matches are found:

```sh
gosimilarity . --fail-on-duplicates
```

By default, matches do not cause failure. Exit codes are `0` for a complete
inspection, `1` for matches with `--fail-on-duplicates`, and `2` for invalid
arguments or an incomplete inspection. See the [CLI reference](docs/cli.md)
for all options, file selection rules, and the JSON format.

## Interpreting results

- A 100% match means identical normalized syntax, **not equivalent behavior**
  or a guarantee that the code can safely be merged. Review matches before
  refactoring.
- Analysis does not build the project, fetch dependencies, or perform type
  checking. Source files are inspected regardless of build tags.
- Directory scans skip tests, generated files, `vendor`, and symlinks by
  default, and respect Git ignores in Git working trees. Use `--include-tests`
  to include tests, or name individual files explicitly.
- Large scans can produce many comparisons and matches. Restrict paths, raise
  `--min-lines`, or use `--exclude` to focus on useful candidates.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for build, test, and development commands.

## License

[MIT](LICENSE)

## Author

[Songmu](https://github.com/Songmu)
