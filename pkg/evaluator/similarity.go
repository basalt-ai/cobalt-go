package evaluator

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// SimilarityEvaluator computes cosine similarity between the agent output
// and the expected output using TF-IDF word vectors (no external API needed).
type SimilarityEvaluator struct {
	name      string
	field     string  // dataset field to compare against (e.g. "expected" or "expectedOutput")
	threshold float64 // if > 0, binarize: score < threshold → 0, else → 1
}

// SimilarityOptions configures the cosine similarity evaluator.
type SimilarityOptions struct {
	// Field is the dataset Item key containing the reference text.
	// Defaults to "expectedOutput" (also checks "expected").
	Field string
	// Threshold: when set (> 0), similarity scores below this value become 0.0,
	// scores at or above become 1.0.
	Threshold float64
}

// NewSimilarity creates a SimilarityEvaluator.
func NewSimilarity(name string, opts SimilarityOptions) *SimilarityEvaluator {
	if opts.Field == "" {
		opts.Field = "expectedOutput"
	}
	return &SimilarityEvaluator{
		name:      name,
		field:     opts.Field,
		threshold: opts.Threshold,
	}
}

func (s *SimilarityEvaluator) Name() string { return s.name }

func (s *SimilarityEvaluator) Evaluate(_ context.Context, ec EvalContext) (EvalResult, error) {
	// Resolve reference text from item.
	ref := ""
	if v, ok := ec.Item[s.field]; ok {
		ref = fmt.Sprintf("%v", v)
	} else if v, ok := ec.Item["expected"]; ok {
		ref = fmt.Sprintf("%v", v)
	} else if v, ok := ec.Item["expectedOutput"]; ok {
		ref = fmt.Sprintf("%v", v)
	}

	if ref == "" || ec.Output == "" {
		return EvalResult{Score: 0, Reason: "empty reference or output"}, nil
	}

	score := cosineSimilarity(ec.Output, ref)

	if s.threshold > 0 {
		if score >= s.threshold {
			return EvalResult{Score: 1, Reason: fmt.Sprintf("similarity %.3f >= threshold %.3f", score, s.threshold)}, nil
		}
		return EvalResult{Score: 0, Reason: fmt.Sprintf("similarity %.3f < threshold %.3f", score, s.threshold)}, nil
	}

	return EvalResult{
		Score:  score,
		Reason: fmt.Sprintf("cosine similarity: %.3f", score),
	}, nil
}

// tokenize splits text into lowercase words and builds a term-frequency map.
func tokenize(text string) map[string]float64 {
	tf := make(map[string]float64)
	words := strings.Fields(strings.ToLower(text))
	for _, w := range words {
		// Remove punctuation
		w = strings.Trim(w, `.,!?;:"'()[]{}`)
		if w != "" {
			tf[w]++
		}
	}
	return tf
}

// cosineSimilarity computes the cosine similarity between two text strings
// using simple term-frequency vectors (no IDF weighting for brevity).
func cosineSimilarity(a, b string) float64 {
	va := tokenize(a)
	vb := tokenize(b)

	// Build vocabulary union.
	vocab := make(map[string]struct{})
	for w := range va {
		vocab[w] = struct{}{}
	}
	for w := range vb {
		vocab[w] = struct{}{}
	}

	if len(vocab) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for w := range vocab {
		fa := va[w]
		fb := vb[w]
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
