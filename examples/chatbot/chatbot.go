package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Maya is a fake customer support chatbot for Acme Corp.
// She's friendly, helpful, and occasionally escalates.

const mayaSystemPrompt = `You are Maya, a friendly and helpful customer support assistant for Acme Corp.

You help customers with:
- Product questions and troubleshooting
- Billing and account issues
- Shipping and returns
- General company information

Guidelines:
- Be warm, empathetic, and professional
- Give concise but complete answers
- If a problem is complex or the customer is upset, offer to escalate to a human agent
- Never make up information you don't know

Company facts:
- Acme Corp was founded in 1987
- Return policy: 30 days, no questions asked
- Shipping: 3-5 business days standard, 1-2 days express
- Support hours: 9am-6pm EST, Monday-Friday`

// AskMaya sends a customer message to Maya and returns her response.
// Uses OpenAI if OPENAI_API_KEY is set; otherwise returns a canned response.
func AskMaya(ctx context.Context, customerMessage string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fakeMayaResponse(customerMessage), nil
	}
	return callMaya(ctx, apiKey, customerMessage)
}

func callMaya(ctx context.Context, apiKey, message string) (string, error) {
	reqBody := map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]any{
			{"role": "system", "content": mayaSystemPrompt},
			{"role": "user", "content": message},
		},
		"max_tokens":  300,
		"temperature": 0.7,
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
		return "", fmt.Errorf("decode: %w", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("openai: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}

// fakeMayaResponse returns a deterministic canned response for testing
// without an API key.
func fakeMayaResponse(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "return") || strings.Contains(lower, "refund"):
		return "Hi! I'd be happy to help with your return. Our policy allows returns within 30 days of purchase, no questions asked. Simply ship the item back with your order number and we'll process your refund within 3-5 business days. Is there anything else I can help you with?"
	case strings.Contains(lower, "shipping") || strings.Contains(lower, "delivery"):
		return "Great question! Standard shipping takes 3-5 business days, and we also offer express shipping (1-2 business days) for an additional fee. You can track your order using the link in your confirmation email. Let me know if you need anything else!"
	case strings.Contains(lower, "angry") || strings.Contains(lower, "terrible") || strings.Contains(lower, "awful") || strings.Contains(lower, "furious"):
		return "I'm so sorry to hear about your experience — that's absolutely not the standard we hold ourselves to. I'd like to escalate this to one of our senior support specialists who can make this right for you. Can I have your order number?"
	case strings.Contains(lower, "hours") || strings.Contains(lower, "available"):
		return "Our support team is available Monday through Friday, 9am to 6pm EST. For urgent issues outside those hours, you can email support@acme.com and we'll get back to you first thing. Is there anything I can help you with right now?"
	case strings.Contains(lower, "founded") || strings.Contains(lower, "history") || strings.Contains(lower, "about"):
		return "Acme Corp was founded in 1987 with a mission to deliver quality products and exceptional service. We've been serving customers for over 35 years! Is there something specific about our company you'd like to know?"
	default:
		return "Thanks for reaching out to Acme Corp support! I'm Maya, and I'm here to help. Could you give me a bit more detail about your question or issue? I want to make sure I give you the most accurate information possible."
	}
}
