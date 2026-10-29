package gosimilarity

import (
	"io"
	"reflect"
	"testing"
)

func TestInterspersedOptions(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args, paths []string
		filter      string
		print       bool
	}{
		{"interspersed", []string{"a.go", "-p", "b.go", "--filter-function", "Target"}, []string{"a.go", "b.go"}, "Target", true},
		{"option-like value", []string{"--filter-function", "--types", "a.go"}, []string{"a.go"}, "--types", false},
		{"equals and terminator", []string{"--filter-function=-value", "--", "-p", "a.go"}, []string{"-p", "a.go"}, "-value", false},
		{"single dash and false", []string{"-", "--print=false", "a.go"}, []string{"-", "a.go"}, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o, paths, ver, err := parseOptions(tt.args, io.Discard)
			if err != nil || ver || !reflect.DeepEqual(paths, tt.paths) ||
				o.filterFunction != tt.filter || o.print != tt.print || !o.noTypes {
				t.Fatalf("options=%+v paths=%v version=%v error=%v", o, paths, ver, err)
			}
		})
	}
}

func TestPackageOptions(t *testing.T) {
	o, paths, _, err := parseOptions([]string{"./cmd/...", "--packages", "--tags=integration,netgo", "./internal/..."}, io.Discard)
	if err != nil || !o.packages || !o.tagsSet || o.tags != "integration,netgo" ||
		!reflect.DeepEqual(paths, []string{"./cmd/...", "./internal/..."}) {
		t.Fatalf("options=%+v paths=%v error=%v", o, paths, err)
	}
	o, paths, _, err = parseOptions([]string{"--packages", "--tags="}, io.Discard)
	if err != nil || !o.packages || !o.tagsSet || o.tags != "" || !reflect.DeepEqual(paths, []string{"."}) {
		t.Fatalf("options=%+v paths=%v error=%v", o, paths, err)
	}
}
