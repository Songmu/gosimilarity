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

gosimilarity short description

## Synopsis

```go
// simple usage here
```

## Description

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

After changing GitHub Actions dependencies, run `gh actions-lock` and commit
`.github/workflows/actions.lock` with the workflow changes. Install the tool
with `gh extension install github/gh-actions-lock`.

## Author

[Songmu](https://github.com/Songmu)
