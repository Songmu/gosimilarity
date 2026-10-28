package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMainProcess(t *testing.T) {
	if os.Getenv("GOSIMILARITY_TEST_MAIN") != "1" {
		return
	}
	args := os.Args[slices.Index(os.Args, "--")+1:]
	os.Args = append(os.Args[:1], args...)
	main()
	os.Exit(0)
}

func TestCLI(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "example.go"), []byte("package example\ntype A int\ntype B int\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name        string
		args        []string
		code        int
		out, stderr string
	}{
		{"success", []string{"example.go"}, 0, "Status: complete;", ""},
		{"help", []string{"--help"}, 0, "", "Usage: gosimilarity"},
		{"duplicates", []string{"example.go", "--types", "--fail-on-duplicates"}, 1, "Similarity:", "similar declarations found"},
		{"invalid arguments", []string{"--format", "yaml"}, 2, "Status: incomplete;", "--format must be text or json"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(executable, append([]string{"-test.run=^TestMainProcess$", "--"}, tt.args...)...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOSIMILARITY_TEST_MAIN=1")
			var out, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &stderr
			code := 0
			if err := cmd.Run(); err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			stdoutMatches := out.Len() == 0
			if tt.out != "" {
				stdoutMatches = strings.Contains(out.String(), tt.out)
			}
			stderrMatches := stderr.Len() == 0
			if tt.stderr != "" {
				stderrMatches = strings.Contains(stderr.String(), tt.stderr)
			}
			if code != tt.code || !stdoutMatches || !stderrMatches {
				t.Fatalf("exit=%d, stdout=%q, stderr=%q; want exit=%d, stdout matching %q, stderr matching %q",
					code, &out, &stderr, tt.code, tt.out, tt.stderr)
			}
		})
	}
}
