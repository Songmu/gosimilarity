# Contributing

Use the Go version declared in [go.mod](go.mod) or later.

## Build and test

Build the CLI from the repository root:

```sh
go build ./cmd/gosimilarity
```

Run the tests and static checks:

```sh
go test -race ./...
go vet ./...
```

Measure coverage or run benchmarks when relevant to your change:

```sh
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go test -run '^$' -bench . -benchmem
```

Keep tests focused on observable behavior and include regression coverage for
bug fixes. Update the [CLI reference](docs/cli.md) when changing options,
file selection, or output.

## GitHub Actions dependencies

After changing GitHub Actions dependencies, run `gh actions-lock` and commit
`.github/workflows/actions.lock` with the workflow changes. Install the tool
with `gh extension install github/gh-actions-lock`.
