// Package mcp implements a minimal MCP server over stdio (JSON-RPC 2.0).
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/basalt-ai/cobalt-go/pkg/experiment"
)

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPTool describes a tool exposed by the MCP server.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server is the MCP server.
type Server struct {
	in  io.Reader
	out io.Writer
}

// New creates a new MCP server reading from in and writing to out.
func New(in io.Reader, out io.Writer) *Server {
	return &Server{in: in, out: out}
}

// Serve reads JSON-RPC requests line-by-line and writes responses.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error")
			continue
		}

		resp := s.handle(ctx, &req)
		s.write(resp)
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "ping":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32601, Message: "method not found: " + req.Method},
		}
	}
}

func (s *Server) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "cobalt",
				"version": "0.1.0",
			},
		},
	}
}

func (s *Server) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	tools := []MCPTool{
		{
			Name:        "cobalt_run",
			Description: "Build and run a Cobalt experiment Go file, returning the report.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file": map[string]any{"type": "string", "description": "Path to the experiment .go file"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "cobalt_results",
			Description: "List recent experiment runs or get details for a specific run by ID.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":    map[string]any{"type": "string", "description": "Run ID (or prefix) to fetch specific result"},
					"limit": map[string]any{"type": "integer", "description": "Max results to list (default: 10)"},
				},
			},
		},
		{
			Name:        "cobalt_compare",
			Description: "Compare two experiment runs side by side.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id1": map[string]any{"type": "string", "description": "First run ID"},
					"id2": map[string]any{"type": "string", "description": "Second run ID"},
				},
				"required": []string{"id1", "id2"},
			},
		},
		{
			Name:        "cobalt_generate",
			Description: "Generate a Cobalt experiment file using OpenAI based on a description.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]any{"type": "string", "description": "What the experiment should test"},
					"outputFile":  map[string]any{"type": "string", "description": "File path to write the generated experiment"},
				},
				"required": []string{"description", "outputFile"},
			},
		},
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": tools},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResp(req.ID, -32602, "invalid params")
	}

	var result any
	var toolErr error

	switch params.Name {
	case "cobalt_run":
		result, toolErr = s.toolRun(ctx, params.Arguments)
	case "cobalt_results":
		result, toolErr = s.toolResults(params.Arguments)
	case "cobalt_compare":
		result, toolErr = s.toolCompare(params.Arguments)
	case "cobalt_generate":
		result, toolErr = s.toolGenerate(ctx, params.Arguments)
	default:
		return s.errorResp(req.ID, -32601, "unknown tool: "+params.Name)
	}

	if toolErr != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": fmt.Sprintf("Error: %v", toolErr)},
				},
				"isError": true,
			},
		}
	}

	text, _ := json.MarshalIndent(result, "", "  ")
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(text)},
			},
		},
	}
}

func (s *Server) toolRun(ctx context.Context, args map[string]any) (any, error) {
	file, _ := args["file"].(string)
	if file == "" {
		return nil, fmt.Errorf("file argument required")
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "run", file)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go run failed: %v\nstderr: %s", err, stderr.String())
	}

	return map[string]any{
		"output": stdout.String(),
		"stderr": stderr.String(),
		"status": "ok",
	}, nil
}

func (s *Server) toolResults(args map[string]any) (any, error) {
	if id, ok := args["id"].(string); ok && id != "" {
		return experiment.LoadReport(id)
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}
	return experiment.ListHistory(limit)
}

func (s *Server) toolCompare(args map[string]any) (any, error) {
	id1, _ := args["id1"].(string)
	id2, _ := args["id2"].(string)
	if id1 == "" || id2 == "" {
		return nil, fmt.Errorf("id1 and id2 required")
	}

	r1, err := experiment.LoadReport(id1)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", id1, err)
	}
	r2, err := experiment.LoadReport(id2)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", id2, err)
	}

	deltas := make(map[string]float64)
	for k, sa := range r1.Scores {
		if sb, ok := r2.Scores[k]; ok {
			deltas[k] = sb.Avg - sa.Avg
		}
	}

	return map[string]any{
		"a":      map[string]any{"id": r1.ID, "name": r1.Name, "scores": r1.Scores},
		"b":      map[string]any{"id": r2.ID, "name": r2.Name, "scores": r2.Scores},
		"deltas": deltas,
	}, nil
}

func (s *Server) toolGenerate(ctx context.Context, args map[string]any) (any, error) {
	description, _ := args["description"].(string)
	outputFile, _ := args["outputFile"].(string)
	if description == "" || outputFile == "" {
		return nil, fmt.Errorf("description and outputFile required")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	prompt := fmt.Sprintf(`Generate a complete Go experiment file for the Cobalt AI evaluation framework.

Description: %s

The file should:
1. Be a valid Go program (package main)
2. Import github.com/basalt-ai/cobalt-go/pkg/dataset, experiment, and evaluator packages
3. Define a dataset with 5-10 relevant test items
4. Use appropriate evaluators (FunctionEvaluator, LLMJudge, or SimilarityEvaluator)
5. Call experiment.Run() and print the report

Output only the Go source code, no explanation.`, description)

	content, err := openAIGenerate(ctx, apiKey, prompt)
	if err != nil {
		return nil, err
	}

	// Strip markdown code fences if present.
	if idx := strings.Index(content, "```go"); idx >= 0 {
		content = content[idx+5:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	} else if idx := strings.Index(content, "```"); idx >= 0 {
		content = content[idx+3:]
		if end := strings.Index(content, "```"); end >= 0 {
			content = content[:end]
		}
	}

	if err := os.WriteFile(outputFile, []byte(strings.TrimSpace(content)+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", outputFile, err)
	}

	return map[string]any{
		"outputFile": outputFile,
		"message":    fmt.Sprintf("Generated experiment written to %s", outputFile),
	}, nil
}

// openAIGenerate calls the OpenAI chat completions endpoint for code generation.
func openAIGenerate(ctx context.Context, apiKey, prompt string) (string, error) {
	reqBody := map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]any{
			{"role": "system", "content": "You are an expert Go developer who writes Cobalt AI evaluation experiments."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
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
		return "", fmt.Errorf("empty response from openai")
	}
	return result.Choices[0].Message.Content, nil
}

func (s *Server) write(resp *jsonRPCResponse) {
	b, _ := json.Marshal(resp)
	fmt.Fprintf(s.out, "%s\n", b)
}

func (s *Server) writeError(id any, code int, msg string) {
	s.write(&jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: msg},
	})
}

func (s *Server) errorResp(id any, code int, msg string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonRPCError{Code: code, Message: msg},
	}
}
