# Changelog

## [v0.0.5](https://github.com/Songmu/gosimilarity/compare/v0.0.4...v0.0.5) - 2026-10-31

- Establish a provisional Go similarity quality evaluation baseline by @Songmu in https://github.com/Songmu/gosimilarity/pull/25
- Reduce comparison enumeration with exact-tree groups and size bounds by @Songmu in https://github.com/Songmu/gosimilarity/pull/26
- Evaluate standard ordered tree edit distance before changing scoring by @Songmu in https://github.com/Songmu/gosimilarity/pull/24
- Compute exact ordered tree edit distance for production similarity by @Songmu in https://github.com/Songmu/gosimilarity/pull/28

## [v0.0.4](https://github.com/Songmu/gosimilarity/compare/v0.0.3...v0.0.4) - 2026-10-30

- Update flagx to v0.0.2 and simplify float flags by @Songmu in https://github.com/Songmu/gosimilarity/pull/22

## [v0.0.3](https://github.com/Songmu/gosimilarity/compare/v0.0.2...v0.0.3) - 2026-10-29

- Use Songmu/flagx for flag aliases and help by @Songmu in https://github.com/Songmu/gosimilarity/pull/20

## [v0.0.2](https://github.com/Songmu/gosimilarity/compare/v0.0.1...v0.0.2) - 2026-10-29

- Add build-aware package selection with --packages by @Songmu in https://github.com/Songmu/gosimilarity/pull/19

## [v0.0.1](https://github.com/Songmu/gosimilarity/commits/v0.0.1) - 2026-10-28

- Bump reviewdog/action-misspell from 1.30.2 to 1.30.3 by @dependabot[bot] in https://github.com/Songmu/gosimilarity/pull/4
- Bump reviewdog/action-actionlint from 1.80.4 to 1.80.6 by @dependabot[bot] in https://github.com/Songmu/gosimilarity/pull/3
- Bump Songmu/tagpr from 1.21.3 to 1.21.4 by @dependabot[bot] in https://github.com/Songmu/gosimilarity/pull/2
- Implement Go structural similarity detection for refactoring discovery by @Songmu in https://github.com/Songmu/gosimilarity/pull/5
- Reduce false-positive type matches with name-aware compact syntax trees by @Songmu in https://github.com/Songmu/gosimilarity/pull/6
- Make type comparisons opt-in to reduce default noise by @Songmu in https://github.com/Songmu/gosimilarity/pull/7
- Isolate interspersed option parsing and raw-literal recovery by @Songmu in https://github.com/Songmu/gosimilarity/pull/8
- Separate pair selection, scoring, and report finalization by @Songmu in https://github.com/Songmu/gosimilarity/pull/9
- Separate file collection from discovery candidate validation by @Songmu in https://github.com/Songmu/gosimilarity/pull/10
- Keep inexpensive candidate checks in the comparison hot loop by @Songmu in https://github.com/Songmu/gosimilarity/pull/14
- Organize regression tests and cover rendering and CLI exits by @Songmu in https://github.com/Songmu/gosimilarity/pull/12
- Separate specialized binding rules from AST dispatch by @Songmu in https://github.com/Songmu/gosimilarity/pull/11
- Share text source rendering and separate output units by @Songmu in https://github.com/Songmu/gosimilarity/pull/13
- Remove redundant binding and glob memoization state by @Songmu in https://github.com/Songmu/gosimilarity/pull/15
- docs: prepare user-facing documentation for public release by @Songmu in https://github.com/Songmu/gosimilarity/pull/16
- docs: retain bounded benchmark runs in contributor instructions by @Songmu in https://github.com/Songmu/gosimilarity/pull/17
