package evaluator

import (
	"context"
	"strings"
	"testing"
)

// --- FunctionEvaluator tests ---

func TestFunctionEvaluator_Name(t *testing.T) {
	e := NewFunction("my-eval", func(ec EvalContext) (EvalResult, error) {
		return EvalResult{Score: 1}, nil
	})
	if e.Name() != "my-eval" {
		t.Fatalf("expected name 'my-eval', got %q", e.Name())
	}
}

func TestFunctionEvaluator_Score(t *testing.T) {
	e := NewFunction("exact-match", func(ec EvalContext) (EvalResult, error) {
		if ec.Output == ec.Item["expected"] {
			return EvalResult{Score: 1, Reason: "exact match"}, nil
		}
		return EvalResult{Score: 0, Reason: "no match"}, nil
	})

	res, err := e.Evaluate(context.Background(), EvalContext{
		Item:   map[string]any{"expected": "hello"},
		Output: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 1 {
		t.Fatalf("expected score 1, got %f", res.Score)
	}
}

func TestFunctionEvaluator_ClampScore(t *testing.T) {
	e := NewFunction("over-scorer", func(ec EvalContext) (EvalResult, error) {
		return EvalResult{Score: 2.5}, nil
	})
	res, err := e.Evaluate(context.Background(), EvalContext{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 1.0 {
		t.Fatalf("expected clamped score 1.0, got %f", res.Score)
	}
}

func TestFunctionEvaluator_ClampNegative(t *testing.T) {
	e := NewFunction("under-scorer", func(ec EvalContext) (EvalResult, error) {
		return EvalResult{Score: -0.5}, nil
	})
	res, err := e.Evaluate(context.Background(), EvalContext{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 0.0 {
		t.Fatalf("expected clamped score 0.0, got %f", res.Score)
	}
}

// --- SimilarityEvaluator tests ---

func TestSimilarity_IdenticalText(t *testing.T) {
	e := NewSimilarity("sim", SimilarityOptions{})
	res, err := e.Evaluate(context.Background(), EvalContext{
		Item:   map[string]any{"expectedOutput": "the quick brown fox"},
		Output: "the quick brown fox",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score < 0.99 {
		t.Fatalf("identical text should score ~1.0, got %f", res.Score)
	}
}

func TestSimilarity_CompletelyDifferent(t *testing.T) {
	e := NewSimilarity("sim", SimilarityOptions{})
	res, err := e.Evaluate(context.Background(), EvalContext{
		Item:   map[string]any{"expectedOutput": "the quick brown fox"},
		Output: "zebra unicorn rainbow sparkle",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score > 0.1 {
		t.Fatalf("completely different text should score ~0, got %f", res.Score)
	}
}

func TestSimilarity_Threshold(t *testing.T) {
	e := NewSimilarity("sim-thresh", SimilarityOptions{Threshold: 0.8})
	// High similarity → score 1
	res, err := e.Evaluate(context.Background(), EvalContext{
		Item:   map[string]any{"expectedOutput": "hello world"},
		Output: "hello world test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 1.0 {
		t.Fatalf("expected binarized score 1.0, got %f", res.Score)
	}
}

func TestSimilarity_EmptyInput(t *testing.T) {
	e := NewSimilarity("sim", SimilarityOptions{})
	res, err := e.Evaluate(context.Background(), EvalContext{
		Item:   map[string]any{"expectedOutput": "expected"},
		Output: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 0 {
		t.Fatalf("empty output should score 0, got %f", res.Score)
	}
}

// --- LLMJudge prompt template tests ---

func TestLLMJudge_PromptRendering(t *testing.T) {
	j := NewLLMJudge("test", LLMJudgeOptions{
		Prompt: "Input: {{input}}\nOutput: {{output}}\nExpected: {{expectedOutput}}",
	})
	ec := EvalContext{
		Item:   map[string]any{"input": "hello", "expectedOutput": "world"},
		Output: "world",
	}
	rendered, err := j.renderPrompt(ec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "hello") {
		t.Error("rendered prompt missing input")
	}
	if !strings.Contains(rendered, "world") {
		t.Error("rendered prompt missing output/expected")
	}
}

func TestLLMJudge_ExtractJSON(t *testing.T) {
	raw := `Sure! Here is the evaluation: {"verdict": true, "reasoning": "looks good"} Hope that helps!`
	extracted := extractJSON(raw)
	if !strings.Contains(extracted, "verdict") {
		t.Fatalf("failed to extract JSON: got %q", extracted)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		want float64
		cmp  string
	}{
		{"hello world", "hello world", 0.999, ">="},
		{"", "hello", 0.0, "=="},
		{"hello world", "completely different sentence", 0.0, "<=0.2"},
	}
	for _, tt := range tests {
		got := cosineSimilarity(tt.a, tt.b)
		switch tt.cmp {
		case ">=":
			if got < tt.want {
				t.Errorf("cosineSimilarity(%q,%q) = %f, want >= %f", tt.a, tt.b, got, tt.want)
			}
		case "==":
			if got != tt.want {
				t.Errorf("cosineSimilarity(%q,%q) = %f, want %f", tt.a, tt.b, got, tt.want)
			}
		case "<=0.2":
			if got > 0.2 {
				t.Errorf("cosineSimilarity(%q,%q) = %f, want <= 0.2", tt.a, tt.b, got)
			}
		}
	}
}
