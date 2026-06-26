// Package codemetrics computes per-function complexity metrics from source
// code: McCabe cyclomatic complexity and SonarSource cognitive complexity.
//
// Cyclomatic complexity counts decision points flatly (1 + one per branch:
// if / for / range / case / comm-clause / && / ||). Cognitive complexity
// weights *nested* control flow more heavily, so deeply-nested logic scores
// higher than a flat sequence of the same number of branches — it tracks how
// hard code is to understand rather than how many paths it has.
//
// The Go analyzer is built on the standard library's go/ast and has no
// external dependencies. The cognitive-complexity algorithm follows the
// SonarSource specification (reference implementation:
// github.com/uudashr/gocognit).
//
// The public API is shaped so additional languages can be added behind
// [Parse] without breaking callers; today only Go is supported and
// [FunctionMetrics.Cognitive] is always populated. For languages that do not
// (yet) support cognitive complexity, Cognitive is nil — distinct from a
// genuine zero.
package codemetrics
