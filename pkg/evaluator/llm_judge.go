package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"
)

// ScoringMode controls how the LLM judge returns its score.
type ScoringMode string

const (
	// ScoringBoolean returns 0 or 1.
	ScoringBoolean ScoringMode = "boolean"
	// ScoringScale returns a float in [0.0, 1.0].
	ScoringScale ScoringMode = "scale"
)

// LLMJudgeOptions configures an LLMJudge evaluator.
type LLMJudgeOptions struct {
	// Prompt is a Go template with {{.Input}}, {{.Output}}, {{.ExpectedOutput}} variables.
	// Also supports {{.Item.<field>}} for arbitrary item fields.
	Prompt string
	// Scoring is "boolean" (default) or "scale".
	Scoring ScoringMode
	// ChainOfThought requests a reasoning explanation before the verdict.
	ChainOfThought bool
	// Model is the OpenAI model to use (default: gpt-4o-mini).
	Model string
	// APIKey overrides OPENAI_API_KEY environment variable.
	APIKey string
}

// LLMJudge calls an OpenAI-compatible API to score outputs.
type LLMJudge struct {
	name string
	opts LLMJudgeOptions
}

// NewLLMJudge creates a new LLMJudge evaluator.
func NewLLMJudge(name string, opts LLMJudgeOptions) *LLMJudge {
	if opts.Model == "" {
		opts.Model = "gpt-4o-mini"
	}
	if opts.Scoring == "" {
		opts.Scoring = ScoringBoolean
	}
	return &LLMJudge{name: name, opts: opts}
}

func (j *LLMJudge) Name() string { return j.name }

func (j *LLMJudge) Evaluate(ctx context.Context, ec EvalContext) (EvalResult, error) {
	prompt, err := j.renderPrompt(ec)
	if err != nil {
		return EvalResult{}, fmt.Errorf("llm_judge %s: render prompt: %w", j.name, err)
	}

	systemMsg := j.buildSystemPrompt()
	apiKey := j.opts.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return EvalResult{}, fmt.Errorf("llm_judge %s: OPENAI_API_KEY not set", j.name)
	}

	content, err := callOpenAI(ctx, apiKey, j.opts.Model, systemMsg, prompt)
	if err != nil {
		return EvalResult{}, fmt.Errorf("llm_judge %s: api call: %w", j.name, err)
	}

	return j.parseResponse(content)
}

func (j *LLMJudge) buildSystemPrompt() string {
	if j.opts.Scoring == ScoringScale {
		if j.opts.ChainOfThought {
			return `You are an expert evaluator. Analyze the task and respond with JSON: {"reasoning": "...", "score": 0.75}. Score must be a float between 0.0 and 1.0.`
		}
		return `You are an expert evaluator. Respond with JSON: {"score": 0.75}. Score must be a float between 0.0 and 1.0.`
	}
	// boolean
	if j.opts.ChainOfThought {
		return `You are an expert evaluator. Analyze the task and respond with JSON: {"reasoning": "...", "verdict": true}. verdict must be a boolean.`
	}
	return `You are an expert evaluator. Respond with JSON: {"verdict": true}. verdict must be a boolean.`
}

// templateData is passed into the prompt template.
type templateData struct {
	Input          string
	Output         string
	ExpectedOutput string
	Item           map[string]any
	Metadata       map[string]any
}

// doublebraceRe matches {{variable}} placeholders (non-template syntax).
var doublebraceRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// renderPrompt renders the judge prompt using Go text/template.
// It first converts {{variable}} → {{.Variable}} for convenience.
func (j *LLMJudge) renderPrompt(ec EvalContext) (string, error) {
	// Convert simple {{variable}} placeholders to Go template syntax.
	promptStr := doublebraceRe.ReplaceAllStringFunc(j.opts.Prompt, func(m string) string {
		inner := m[2 : len(m)-2]
		switch strings.ToLower(inner) {
		case "input":
			return "{{.Input}}"
		case "output":
			return "{{.Output}}"
		case "expectedoutput", "expected_output":
			return "{{.ExpectedOutput}}"
		default:
			return fmt.Sprintf(`{{index .Item %q}}`, inner)
		}
	})

	tmpl, err := template.New("prompt").Parse(promptStr)
	if err != nil {
		return "", err
	}

	expectedOutput := ""
	if v, ok := ec.Item["expectedOutput"]; ok {
		expectedOutput = fmt.Sprintf("%v", v)
	} else if v, ok := ec.Item["expected_output"]; ok {
		expectedOutput = fmt.Sprintf("%v", v)
	} else if v, ok := ec.Item["expected"]; ok {
		expectedOutput = fmt.Sprintf("%v", v)
	}

	input := ""
	if v, ok := ec.Item["input"]; ok {
		input = fmt.Sprintf("%v", v)
	}

	data := templateData{
		Input:          input,
		Output:         ec.Output,
		ExpectedOutput: expectedOutput,
		Item:           ec.Item,
		Metadata:       ec.Metadata,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (j *LLMJudge) parseResponse(content string) (EvalResult, error) {
	// Extract JSON block from response (model may add explanation text).
	content = extractJSON(content)

	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return EvalResult{}, fmt.Errorf("parse llm response: %w (got: %s)", err, content)
	}

	reason := ""
	if r, ok := raw["reasoning"].(string); ok {
		reason = r
	}

	if j.opts.Scoring == ScoringScale {
		score, ok := raw["score"].(float64)
		if !ok {
			return EvalResult{}, fmt.Errorf("missing 'score' field in response")
		}
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}
		return EvalResult{Score: score, Reason: reason}, nil
	}

	// boolean
	verdict, ok := raw["verdict"].(bool)
	if !ok {
		return EvalResult{}, fmt.Errorf("missing 'verdict' field in response")
	}
	score := 0.0
	if verdict {
		score = 1.0
	}
	return EvalResult{Score: score, Reason: reason}, nil
}

// extractJSON pulls the first JSON object out of an arbitrary string response.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return s
	}
	return s[start : end+1]
}

// openAIRequest mirrors the OpenAI chat completions request structure.
type openAIRequest struct {
	Model    string              `json:"model"`
	Messages []openAIMessage     `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// callOpenAI sends a chat completion request to the OpenAI API.
func callOpenAI(ctx context.Context, apiKey, model, system, user string) (string, error) {
	reqBody := openAIRequest{
		Model: model,
		Messages: []openAIMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

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

	var oaiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if oaiResp.Error != nil {
		return "", fmt.Errorf("openai error: %s", oaiResp.Error.Message)
	}
	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("empty choices in response")
	}
	return oaiResp.Choices[0].Message.Content, nil
}
