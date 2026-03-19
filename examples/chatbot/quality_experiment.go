package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/basalt-ai/cobalt-go/pkg/dataset"
	"github.com/basalt-ai/cobalt-go/pkg/evaluator"
	"github.com/basalt-ai/cobalt-go/pkg/experiment"
)

// qualityDataset contains 10 customer support scenarios for Maya.
var qualityDataset = dataset.New([]dataset.Item{
	{
		"input":          "What is your return policy?",
		"expectedOutput": "30 days",
		"category":       "returns",
	},
	{
		"input":          "How long does shipping take?",
		"expectedOutput": "3-5 business days",
		"category":       "shipping",
	},
	{
		"input":          "I want to return a product I bought 3 weeks ago",
		"expectedOutput": "help with return",
		"category":       "returns",
	},
	{
		"input":          "When was Acme Corp founded?",
		"expectedOutput": "1987",
		"category":       "company",
	},
	{
		"input":          "What are your support hours?",
		"expectedOutput": "9am-6pm EST",
		"category":       "support",
	},
	{
		"input":          "I'm furious! My order arrived completely broken!",
		"expectedOutput": "escalate",
		"category":       "escalation",
	},
	{
		"input":          "Do you offer express shipping?",
		"expectedOutput": "1-2 business days",
		"category":       "shipping",
	},
	{
		"input":          "How do I track my order?",
		"expectedOutput": "confirmation email",
		"category":       "shipping",
	},
	{
		"input":          "I've been a customer for 10 years and this is terrible service!",
		"expectedOutput": "escalate",
		"category":       "escalation",
	},
	{
		"input":          "What's your email for support?",
		"expectedOutput": "support@acme.com",
		"category":       "support",
	},
})

// runQualityExperiment evaluates Maya's response quality using 3 evaluators.
func runQualityExperiment(ctx context.Context) (*experiment.Report, error) {
	fmt.Println("\n=== Quality Experiment: Maya Support Bot ===")

	// Evaluator 1: LLM Judge — correctness
	correctnessJudge := evaluator.NewLLMJudge("correctness", evaluator.LLMJudgeOptions{
		Prompt: `You are evaluating a customer support chatbot response.

Customer question: {{input}}
Bot response: {{output}}
Expected to contain: {{expectedOutput}}

Does the response correctly address the customer's question and contain the expected information?
Respond with JSON: {"verdict": true/false, "reasoning": "..."}`,
		Scoring:        evaluator.ScoringBoolean,
		ChainOfThought: true,
	})

	// Evaluator 2: LLM Judge — tone
	toneJudge := evaluator.NewLLMJudge("tone", evaluator.LLMJudgeOptions{
		Prompt: `Rate the tone quality of this customer support response.

Customer message: {{input}}
Bot response: {{output}}

Rate on a scale of 0.0-1.0 how well the response:
- Is warm and empathetic
- Uses professional language
- Is appropriately concise

Respond with JSON: {"score": 0.8, "reasoning": "..."}`,
		Scoring:        evaluator.ScoringScale,
		ChainOfThought: true,
	})

	// Evaluator 3: Function — escalation detection
	escalationEval := evaluator.NewFunction("escalation", func(ec evaluator.EvalContext) (evaluator.EvalResult, error) {
		expectedOutput := fmt.Sprintf("%v", ec.Item["expectedOutput"])
		output := strings.ToLower(ec.Output)

		if expectedOutput == "escalate" {
			// Check if response offers escalation
			escalationKeywords := []string{"escalate", "specialist", "senior", "human", "manager", "supervisor"}
			for _, kw := range escalationKeywords {
				if strings.Contains(output, kw) {
					return evaluator.EvalResult{Score: 1, Reason: "correctly offers escalation"}, nil
				}
			}
			return evaluator.EvalResult{Score: 0, Reason: "should have offered escalation"}, nil
		}
		// Not an escalation scenario — check expected content is present
		expected := strings.ToLower(expectedOutput)
		if strings.Contains(output, expected) {
			return evaluator.EvalResult{Score: 1, Reason: "contains expected content"}, nil
		}
		return evaluator.EvalResult{Score: 0.5, Reason: "partial match"}, nil
	})

	runner := func(ctx context.Context, item map[string]any, index int) (string, error) {
		input := fmt.Sprintf("%v", item["input"])
		return AskMaya(ctx, input)
	}

	apiKey := getOpenAIKey()
	evals := []evaluator.Evaluator{escalationEval}
	if apiKey != "" {
		evals = []evaluator.Evaluator{correctnessJudge, toneJudge, escalationEval}
	} else {
		fmt.Println("Note: OPENAI_API_KEY not set — skipping LLM judges, using function evaluator only")
	}

	report, err := experiment.Run(ctx, "maya-quality", qualityDataset, runner, experiment.Options{
		Evaluators:  evals,
		Concurrency: 3,
		Tags:        []string{"quality", "chatbot", "maya"},
	})
	if err != nil {
		return nil, err
	}

	report.Print()

	if err := experiment.SaveReport(report); err != nil {
		fmt.Printf("Warning: could not save report: %v\n", err)
	}

	return report, nil
}
