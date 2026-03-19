// Package main demonstrates the Cobalt evaluation framework with a
// fake customer support chatbot (Maya from Acme Corp).
//
// Usage:
//
//	go run ./examples/chatbot/
//	OPENAI_API_KEY=sk-... go run ./examples/chatbot/
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	ctx := context.Background()

	fmt.Println("╔═══════════════════════════════════════════╗")
	fmt.Println("║  Cobalt Go — Chatbot Evaluation Demo       ║")
	fmt.Println("╚═══════════════════════════════════════════╝")

	if getOpenAIKey() == "" {
		fmt.Println("\nℹ  No OPENAI_API_KEY — running in offline mode (fake responses, no LLM judges)")
	}

	// Run quality experiment.
	_, err := runQualityExperiment(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Quality experiment failed: %v\n", err)
		os.Exit(1)
	}

	// Run model comparison experiment.
	if err := runCompareExperiment(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Compare experiment failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✓ All experiments complete. Run 'cobalt history' to see saved results.")
}

// getOpenAIKey returns the OPENAI_API_KEY environment variable.
func getOpenAIKey() string {
	return os.Getenv("OPENAI_API_KEY")
}
