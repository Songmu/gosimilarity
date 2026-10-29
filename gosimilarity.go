package gosimilarity

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"strings"
)

const cmdName = "gosimilarity"

type options struct {
	threshold      float64
	renameCost     float64
	minLines       int
	minTokens      int
	minTokensSet   bool
	minLinesSet    bool
	noFunctions    bool
	noTypes        bool
	print          bool
	includeTests   bool
	packages       bool
	tags           string
	tagsSet        bool
	crossFileOnly  bool
	failDuplicates bool
	format         string
	filterFunction string
	filterBody     string
	excludes       []string
}

type exitError struct {
	code    int
	message string
}

func (e *exitError) Error() string { return e.message }
func (e *exitError) ExitCode() int { return e.code }

type patterns []string

func (p *patterns) String() string { return strings.Join(*p, ", ") }
func (p *patterns) Set(v string) error {
	for _, part := range strings.Split(v, "/") {
		if _, err := path.Match(part, ""); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", v, err)
		}
	}
	*p = append(*p, strings.TrimPrefix(v, "./"))
	return nil
}

func parseOptions(argv []string, errStream io.Writer) (options, []string, bool, error) {
	var o options
	var ver bool
	var compareTypes bool
	var excludes patterns
	fs := flag.NewFlagSet(cmdName, flag.ContinueOnError)
	fs.SetOutput(errStream)
	fs.Usage = func() {
		fmt.Fprintln(errStream, "Usage: gosimilarity [options] [files or directories...]\n       gosimilarity --packages [options] [package patterns...]\n\nFind structurally similar Go functions and type declarations.\nWith --packages, . selects one package; ./... selects packages recursively.")
		fs.PrintDefaults()
	}
	fs.Float64Var(&o.threshold, "threshold", 0.87, "similarity threshold (0..1)")
	fs.Float64Var(&o.threshold, "t", 0.87, "alias for --threshold")
	fs.Float64Var(&o.renameCost, "rename-cost", 0.3, "positive node replacement cost")
	fs.Float64Var(&o.renameCost, "r", 0.3, "alias for --rename-cost")
	fs.IntVar(&o.minLines, "min-lines", 3, "minimum function declaration lines")
	fs.IntVar(&o.minLines, "m", 3, "alias for --min-lines")
	fs.IntVar(&o.minTokens, "min-tokens", 0, "minimum normalized function tree nodes (overrides --min-lines)")
	fs.BoolVar(&compareTypes, "types", false, "include type comparisons")
	fs.BoolVar(&o.noFunctions, "no-functions", false, "compare only types")
	fs.BoolVar(&o.noTypes, "no-types", false, "disable type comparisons")
	fs.BoolVar(&o.print, "print", false, "include original declarations")
	fs.BoolVar(&o.print, "p", false, "alias for --print")
	fs.BoolVar(&o.includeTests, "include-tests", false, "include discovered test files")
	fs.BoolVar(&o.packages, "packages", false, "select build-eligible files using Go package patterns")
	fs.StringVar(&o.tags, "tags", "", "additional build tags for --packages (comma-separated)")
	fs.BoolVar(&o.crossFileOnly, "cross-file-only", false, "exclude same-file comparisons")
	fs.BoolVar(&o.failDuplicates, "fail-on-duplicates", false, "exit 1 when matches are found")
	fs.StringVar(&o.format, "format", "text", "output format: text or json")
	fs.StringVar(&o.filterFunction, "filter-function", "", "pair-side function name substring")
	fs.StringVar(&o.filterBody, "filter-function-body", "", "pair-side original body substring")
	fs.Var(&excludes, "exclude", "exclude cwd-relative glob (repeatable; ** matches directories)")
	fs.BoolVar(&ver, "version", false, "display version")
	fs.BoolVar(&ver, "V", false, "alias for --version")
	paths, err := parseInterspersed(fs, argv)
	if err != nil {
		return o, nil, false, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "min-tokens" {
			o.minTokensSet = true
		}
		if f.Name == "min-lines" || f.Name == "m" {
			o.minLinesSet = true
		}
		if f.Name == "tags" {
			o.tagsSet = true
		}
	})
	if o.tagsSet && !o.packages {
		return o, nil, false, errors.New("--tags requires --packages")
	}
	if o.packages {
		for _, input := range paths {
			if strings.HasSuffix(input, ".go") {
				return o, nil, false, errors.New("--packages accepts package patterns, not Go file paths")
			}
		}
	}
	if !finite(o.threshold) || o.threshold < 0 || o.threshold > 1 {
		return o, nil, false, errors.New("--threshold must be finite and between 0 and 1")
	}
	if !finite(o.renameCost) || o.renameCost <= 0 {
		return o, nil, false, errors.New("--rename-cost must be finite and positive")
	}
	if o.minLines < 0 || o.minTokens < 0 {
		return o, nil, false, errors.New("minimum sizes must be nonnegative")
	}
	if o.noFunctions && o.noTypes {
		return o, nil, false, errors.New("--no-functions and --no-types cannot both be set")
	}
	if compareTypes && o.noTypes {
		return o, nil, false, errors.New("--types and --no-types cannot both be set")
	}
	o.noTypes = !compareTypes && !o.noFunctions
	if o.format != "json" && o.format != "text" {
		return o, nil, false, errors.New("--format must be text or json")
	}
	o.excludes = excludes
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return o, paths, ver, nil
}

// Parse one option at a time to support interspersed paths and -- without
// accidentally treating a flag's value as another option.
func parseInterspersed(fs *flag.FlagSet, argv []string) ([]string, error) {
	var paths []string
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			paths = append(paths, argv[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		name, _, equals := strings.Cut(name, "=")
		f := fs.Lookup(name)
		args := []string{arg}
		if f != nil && !equals {
			boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
			if !ok || !boolean.IsBoolFlag() {
				if i+1 < len(argv) {
					i++
					args = append(args, argv[i])
				}
			}
		}
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// Run finds structurally similar Go declarations and writes text or JSON.
// Errors expose ExitCode() int: 1 for requested duplicate failure, 2 for
// invalid arguments or an incomplete inspection. Help returns flag.ErrHelp.
func Run(ctx context.Context, argv []string, outStream, errStream io.Writer) error {
	o, paths, ver, err := parseOptions(argv, errStream)
	if errors.Is(err, flag.ErrHelp) {
		return flag.ErrHelp
	}
	r := newReport()
	if err != nil {
		r.diagnose("error", "arguments", "", "", err.Error())
	} else if ver {
		return printVersion(outStream)
	} else {
		if o.minTokensSet && o.minLinesSet {
			r.diagnose("warning", "arguments", "", "", "--min-tokens overrides --min-lines")
		}
		cwd, e := canonicalPath(".")
		if e != nil {
			r.diagnose("error", "discovery", "", "", e.Error())
		} else {
			analyze(ctx, paths, cwd, o, r)
		}
	}
	if err := logDiagnostics(ctx, errStream, r.Diagnostics); err != nil {
		return &exitError{2, err.Error()}
	}
	if o.format == "json" {
		enc := json.NewEncoder(outStream)
		enc.SetIndent("", "  ")
		err = enc.Encode(r)
	} else {
		err = renderText(outStream, r)
	}
	if err != nil {
		return &exitError{2, err.Error()}
	}
	if r.Status == "incomplete" {
		return &exitError{2, "inspection incomplete"}
	}
	if o.failDuplicates && len(r.Pairs) > 0 {
		return &exitError{1, "similar declarations found"}
	}
	return nil
}

func printVersion(out io.Writer) error {
	_, err := fmt.Fprintf(out, "%s v%s (rev:%s)\n", cmdName, version, revision)
	return err
}
