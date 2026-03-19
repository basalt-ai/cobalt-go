package evaluator

import "context"

// FunctionEvaluator wraps a user-provided function as an Evaluator.
type FunctionEvaluator struct {
	name string
	fn   func(EvalContext) (EvalResult, error)
}

// NewFunction creates a new FunctionEvaluator with the given name and scoring function.
// The function receives an EvalContext and must return a score in [0.0, 1.0].
func NewFunction(name string, fn func(EvalContext) (EvalResult, error)) *FunctionEvaluator {
	return &FunctionEvaluator{name: name, fn: fn}
}

func (f *FunctionEvaluator) Name() string { return f.name }

func (f *FunctionEvaluator) Evaluate(_ context.Context, ec EvalContext) (EvalResult, error) {
	result, err := f.fn(ec)
	if err != nil {
		return EvalResult{}, err
	}
	// Clamp score to [0, 1].
	if result.Score < 0 {
		result.Score = 0
	}
	if result.Score > 1 {
		result.Score = 1
	}
	return result, nil
}
