package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/basalt-ai/cobalt-go/pkg/dataset"
	"github.com/basalt-ai/cobalt-go/pkg/evaluator"
	"github.com/basalt-ai/cobalt-go/pkg/experiment"
)

// compareDataset is a focused set of items for model comparison.
var compareDataset = dataset.New([]dataset.Item{
	{"input": "What is your return policy?", "expectedOutput": "30 days"},
	{"input": "How long does shipping take?", "expectedOutput": "3-5 business days"},
	{"input": "I'm angry about my broken order!", "expectedOutput": "escalate"},
	{"input": "What are your support hours?", "expectedOutput": "9am-6pm EST"},
	{"input": "When was Acme Corp founded?", "expectedOutput": "1987"},
})

// makeModelRunner creates a runner that uses a specific OpenAI model.
func makeModelRunner(model string) experiment.RunnerFunc {
	return func(ctx context.Context, item map[string]any, index int) (string, error) {
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return fakeMayaResponse(fmt.Sprintf("%v", item["input"])), nil
		}

		reqBody := map[string]any{
			"model": model,
			"messages": []map[string]any{
				{"role": "system", "content": mayaSystemPrompt},
				{"role": "user", "content": fmt.Sprintf("%v", item["input"])},
			},
			"max_tokens":  300,
			"temperature": 0.3,
		}

		b, _ := json.Marshal(reqBody)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", err
		}
		if result.Error != nil {
			return "", fmt.Errorf("openai: %s", result.Error.Message)
		}
		if len(result.Choices) == 0 {
			return "", fmt.Errorf("empty response")
		}
		return result.Choices[0].Message.Content, nil
	}
}

// runCompareExperiment compares gpt-4o-mini vs gpt-4o on the same dataset.
func runCompareExperiment(ctx context.Context) error {
	fmt.Println("\n=== Model Comparison: gpt-4o-mini vs gpt-4o ===")

	sim := evaluator.NewSimilarity("similarity", evaluator.SimilarityOptions{
		Field: "expectedOutput",
	})

	evals := []evaluator.Evaluator{sim}

	fmt.Println("Running gpt-4o-mini...")
	report1, err := experiment.Run(ctx, "maya-gpt4o-mini", compareDataset,
		makeModelRunner("gpt-4o-mini"), experiment.Options{
			Evaluators:  evals,
			Concurrency: 2,
			Tags:        []string{"compare", "gpt-4o-mini"},
		})
	if err != nil {
		return fmt.Errorf("gpt-4o-mini run: %w", err)
	}
	if err := experiment.SaveReport(report1); err != nil {
		fmt.Printf("Warning: save report 1: %v\n", err)
	}

	fmt.Println("Running gpt-4o...")
	report2, err := experiment.Run(ctx, "maya-gpt4o", compareDataset,
		makeModelRunner("gpt-4o"), experiment.Options{
			Evaluators:  evals,
			Concurrency: 2,
			Tags:        []string{"compare", "gpt-4o"},
		})
	if err != nil {
		return fmt.Errorf("gpt-4o run: %w", err)
	}
	if err := experiment.SaveReport(report2); err != nil {
		fmt.Printf("Warning: save report 2: %v\n", err)
	}

	// Print individual reports.
	fmt.Println("\n--- gpt-4o-mini Results ---")
	report1.Print()

	fmt.Println("\n--- gpt-4o Results ---")
	report2.Print()

	// Compare.
	fmt.Println("\n--- Comparison Summary ---")
	fmt.Printf("%-20s  %-12s  %-12s  %-10s\n", "Evaluator", "gpt-4o-mini", "gpt-4o", "Delta")
	fmt.Printf("%-20s  %-12s  %-12s  %-10s\n",
		"--------------------", "------------", "------------", "----------")

	for evalName, s1 := range report1.Scores {
		s2 := report2.Scores[evalName]
		delta := s2.Avg - s1.Avg
		fmt.Printf("%-20s  %-12.3f  %-12.3f  %+.3f\n",
			evalName, s1.Avg, s2.Avg, delta)
	}

	return nil
}
