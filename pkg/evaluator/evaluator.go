// Package evaluator defines interfaces and types for scoring LLM outputs.
package evaluator

import "context"

// EvalContext holds all data available to an evaluator during scoring.
type EvalContext struct {
	// Item is the original dataset item (arbitrary key-value map).
	Item map[string]any
	// Output is the string output produced by the agent/runner.
	Output string
	// Metadata holds optional extra data about the run (e.g., model name, latency).
	Metadata map[string]any
}

// EvalResult is the outcome of a single evaluation.
type EvalResult struct {
	// Score is a normalized value in [0.0, 1.0].
	Score float64
	// Reason is a human-readable explanation for the score (optional).
	Reason string
}

// Evaluator is the interface that all evaluators must implement.
type Evaluator interface {
	// Name returns a unique identifier for this evaluator (used as the column name in reports).
	Name() string
	// Evaluate scores the given context and returns a result.
	Evaluate(ctx context.Context, ec EvalContext) (EvalResult, error)
}
