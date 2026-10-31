# Similarity quality evaluation (provisional)

Run the small, labeled function-pair evaluation from the repository root:

```sh
go test -run '^TestSimilarityQualityEvaluation$' -count=1 -v .
```

`quality_fixtures_test.go` contains eight original pairs and concise rationales.
`useful=true` means a reviewer would benefit from a refactoring suggestion, not
necessarily that the functions are behaviorally equivalent. Negative labels
include deceptively similar domain operations and unrelated implementations.
All labels are provisional human judgments pending review after the standard
tree edit distance assessment; this work is intended as a draft baseline.

The harness uses the existing function-body normalizer, distance, and similarity
functions with the CLI's current default threshold and rename cost (printed on
each run). A score at or above the threshold is a predicted match. It reports
each pair's score, distance, node counts, label, and TP/FP/TN/FN outcome, then
aggregate counts, precision `TP/(TP+FP)`, and recall `TP/(TP+FN)`. A zero
denominator is explicitly `n/a (0/0)`, not zero or perfect performance.

False positives and false negatives are reported, not test failures. Only
malformed fixture metadata/source, invalid scores, or harness regressions fail
tests. There are no score goldens or minimum precision/recall requirements.
Do not tune the threshold or relabel fixtures merely to improve these results.

For comparison with a later distance implementation, run the same command on
both revisions and retain the dataset version, settings, per-pair output, and
summary. Keep labels and snippets fixed during algorithm comparisons; changes
to them should be reviewed separately and the dataset version updated.

This is a handpicked diagnostic set, not a representative benchmark or evidence
of statistical significance. Snippets are parsed but not type-checked; external
symbols stand in for application context. The evaluation deliberately measures
pair scoring only, excluding discovery, minimum-size filters, candidate pruning,
ranking, and grouping. It does not evaluate type declarations or refactoring
safety. Final label review and calibration remain separate work.
