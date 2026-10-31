package gosimilarity

import (
	"fmt"
	"io"
	"testing"
)

type qualityMetrics struct {
	tp, fp, tn, fn int
}

func (m *qualityMetrics) observe(useful bool, score, threshold float64) string {
	if score >= threshold {
		if useful {
			m.tp++
			return "TP"
		}
		m.fp++
		return "FP"
	}
	if useful {
		m.fn++
		return "FN"
	}
	m.tn++
	return "TN"
}

func qualityRatio(numerator, denominator int) string {
	if denominator == 0 {
		return "n/a (0/0)"
	}
	return fmt.Sprintf("%.4f (%d/%d)", float64(numerator)/float64(denominator), numerator, denominator)
}

func (m qualityMetrics) String() string {
	return fmt.Sprintf("TP=%d FP=%d TN=%d FN=%d precision=%s recall=%s",
		m.tp, m.fp, m.tn, m.fn,
		qualityRatio(m.tp, m.tp+m.fp), qualityRatio(m.tp, m.tp+m.fn))
}

// This is an observational evaluation, not a quality gate: provisional human
// labels must not be changed merely to make the current scorer pass.
func TestSimilarityQualityEvaluation(t *testing.T) {
	opts, _, _, err := parseOptions(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("provisional dataset=v1 pairs=%d threshold=%g rename-cost=%g",
		len(qualityFixtures), opts.threshold, opts.renameCost)
	var metrics qualityMetrics
	seen := make(map[string]bool)
	for _, fixture := range qualityFixtures {
		if fixture.name == "" || seen[fixture.name] || fixture.rationale == "" {
			t.Fatalf("fixture needs a unique name and rationale: %+v", fixture)
		}
		seen[fixture.name] = true
		ts := newTrees()
		a := normalizeSource(t, ts, fixture.a, false)
		b := normalizeSource(t, ts, fixture.b, false)
		if len(a) != 1 || len(b) != 1 {
			t.Fatalf("%s: expected one function per side, got %d and %d", fixture.name, len(a), len(b))
		}
		d := distance(a[0], b[0], opts.renameCost)
		score := similarity(a[0], b[0], d)
		if !finite(score) || score < 0 || score > 1 {
			t.Fatalf("%s: invalid score %g", fixture.name, score)
		}
		outcome := metrics.observe(fixture.useful, score, opts.threshold)
		t.Logf("%s useful=%t score=%.6f distance=%.6f nodes=%d/%d outcome=%s: %s",
			fixture.name, fixture.useful, score, d, a[0].size, b[0].size, outcome, fixture.rationale)
	}
	t.Log(metrics.String())
}

func TestQualityMetrics(t *testing.T) {
	var metrics qualityMetrics
	for _, sample := range []struct {
		useful  bool
		score   float64
		outcome string
	}{
		{true, .87, "TP"}, // The production threshold is inclusive.
		{false, .9, "FP"},
		{true, .86, "FN"},
		{false, .1, "TN"},
	} {
		if got := metrics.observe(sample.useful, sample.score, .87); got != sample.outcome {
			t.Fatalf("outcome = %s, want %s", got, sample.outcome)
		}
	}
	if want := "TP=1 FP=1 TN=1 FN=1 precision=0.5000 (1/2) recall=0.5000 (1/2)"; metrics.String() != want {
		t.Fatalf("metrics = %s, want %s", metrics, want)
	}
}

func TestQualityMetricsZeroDenominators(t *testing.T) {
	tests := []struct {
		name    string
		metrics qualityMetrics
		want    string
	}{
		{"empty", qualityMetrics{}, "TP=0 FP=0 TN=0 FN=0 precision=n/a (0/0) recall=n/a (0/0)"},
		{"missed positives", qualityMetrics{fn: 2}, "TP=0 FP=0 TN=0 FN=2 precision=n/a (0/0) recall=0.0000 (0/2)"},
		{"only negatives", qualityMetrics{fp: 1, tn: 2}, "TP=0 FP=1 TN=2 FN=0 precision=0.0000 (0/1) recall=n/a (0/0)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metrics.String(); got != tt.want {
				t.Fatalf("metrics = %s, want %s", got, tt.want)
			}
		})
	}
}
