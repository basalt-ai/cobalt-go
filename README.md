# Cobalt Go

> Unit testing framework for AI agents and LLM-powered applications — Go port.

[![CI](https://github.com/basalt-ai/cobalt-go/actions/workflows/ci.yml/badge.svg)](https://github.com/basalt-ai/cobalt-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/basalt-ai/cobalt-go.svg)](https://pkg.go.dev/github.com/basalt-ai/cobalt-go)

Cobalt makes it easy to run structured evaluations against AI pipelines. Write a dataset, run your agent, score the outputs — then track results over time and enforce CI thresholds.

## Installation

```bash
go get github.com/basalt-ai/cobalt-go
```

CLI:

```bash
go install github.com/basalt-ai/cobalt-go/cmd/cobalt@latest
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/basalt-ai/cobalt-go/pkg/dataset"
    "github.com/basalt-ai/cobalt-go/pkg/evaluator"
    "github.com/basalt-ai/cobalt-go/pkg/experiment"
)

func main() {
    ds := dataset.New([]dataset.Item{
        {"input": "What is 2+2?", "expectedOutput": "4"},
        {"input": "Capital of France?", "expectedOutput": "Paris"},
    })

    sim := evaluator.NewSimilarity("similarity", evaluator.SimilarityOptions{})

    report, err := experiment.Run(context.Background(), "my-experiment", ds,
        func(ctx context.Context, item map[string]any, index int) (string, error) {
            return myAgent(ctx, fmt.Sprintf("%v", item["input"]))
        },
        experiment.Options{
            Evaluators:  []evaluator.Evaluator{sim},
            Concurrency: 3,
        },
    )
    if err != nil {
        panic(err)
    }
    report.Print()
}
```

## Dataset

Load test data from JSON, JSONL, CSV, or build inline:

```go
// Inline
ds := dataset.New([]dataset.Item{
    {"input": "hello", "expected": "world"},
})

// From file
ds, err := dataset.FromJSON("testdata/items.json")
ds, err := dataset.FromJSONL("testdata/items.jsonl")
ds, err := dataset.FromCSV("testdata/items.csv")

// Chainable transformations
ds = ds.Filter(func(i dataset.Item) bool { return i["category"] == "urgent" }).
       Sample(50).
       Slice(0, 20)
```

## Evaluators

### LLM Judge

Uses OpenAI to score outputs against a prompt template:

```go
judge := evaluator.NewLLMJudge("correctness", evaluator.LLMJudgeOptions{
    Prompt: `Does this response correctly answer the question?
Question: {{input}}
Response: {{output}}
Expected: {{expectedOutput}}`,
    Scoring:        evaluator.ScoringBoolean, // or ScoringScale
    ChainOfThought: true,
    Model:          "gpt-4o-mini", // default
})
```

### Function Evaluator

Write custom scoring logic in Go:

```go
exact := evaluator.NewFunction("exact-match", func(ec evaluator.EvalContext) (evaluator.EvalResult, error) {
    expected := fmt.Sprintf("%v", ec.Item["expectedOutput"])
    if ec.Output == expected {
        return evaluator.EvalResult{Score: 1, Reason: "exact match"}, nil
    }
    return evaluator.EvalResult{Score: 0}, nil
})
```

### Cosine Similarity

TF-IDF based similarity — no external API required:

```go
sim := evaluator.NewSimilarity("similarity", evaluator.SimilarityOptions{
    Field:     "expectedOutput", // dataset field to compare against
    Threshold: 0.7,              // optional: binarize at threshold
})
```

## Experiment Runner

```go
report, err := experiment.Run(ctx, "my-experiment", ds, runner, experiment.Options{
    Evaluators:  []evaluator.Evaluator{judge, sim, exact},
    Concurrency: 3,           // parallel items (default: 3)
    Timeout:     30 * time.Second, // per-item timeout
    Tags:        []string{"v2", "production"},
    Runs:        3,           // run each item N times, average scores
    Thresholds: map[string]experiment.ThresholdMetric{
        "correctness": {Avg: float64Ptr(0.8)},
    },
})
```

## Reports & History

Results are automatically saved to `~/.cobalt/results/`.

```go
// Save
experiment.SaveReport(report)

// Load by ID
report, err := experiment.LoadReport("abc12345")

// List history
entries, err := experiment.ListHistory(10)
```

## CI Integration

```go
avg := 0.8
pass, failures := report.CIPass(map[string]experiment.ThresholdMetric{
    "correctness": {Avg: &avg},
    "tone":        {P95: float64Ptr(0.7)},
})
if !pass {
    fmt.Println("CI failed:", failures)
    os.Exit(1)
}
```

## CLI

```bash
# Initialize project
cobalt init

# Run experiment
cobalt run ./experiments/quality.go

# View history
cobalt history --limit 10

# Compare two runs
cobalt compare abc12345 def67890

# Start dashboard
cobalt serve --port 3001

# Start MCP server
cobalt mcp
```

## Configuration

`cobalt.yaml` in your project root:

```yaml
judge:
  model: gpt-4o-mini
  provider: openai

concurrency: 3
timeout: 30s
testDir: ./experiments
testMatch:
  - "**/*.cobalt.go"
```

## MCP Server

Cobalt includes an MCP server for integration with AI assistants:

```bash
cobalt mcp
```

Available tools:
- `cobalt_run` — run an experiment file
- `cobalt_results` — list or fetch results
- `cobalt_compare` — compare two runs
- `cobalt_generate` — generate an experiment file with OpenAI

## Example: Chatbot Evaluation

See [`examples/chatbot/`](./examples/chatbot/) for a complete example evaluating a fake customer support bot (Maya) with:
- LLM Judge (correctness + tone)
- Function evaluator (escalation detection)
- Model comparison (gpt-4o-mini vs gpt-4o)

```bash
go run ./examples/chatbot/
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Required for LLM Judge and cobalt_generate |

## License

MIT
