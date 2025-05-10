package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

// MockLLMRequest represents a common structure for LLM API requests
type MockLLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages,omitempty"` // OpenAI style
	Prompt      string    `json:"prompt,omitempty"`   // Anthropic style
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// MockOpenAIServer returns a test server that mimics OpenAI's API
var openAIRequestCount int

func MockOpenAIServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req MockLLMRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Fail with rate limit after first request to trigger fallback
		openAIRequestCount++
		if openAIRequestCount > 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
			return
		}

		response := map[string]interface{}{
			"id":     "mock-completion-id",
			"object": "chat.completion",
			"model":  req.Model,
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Mock response for: " + req.Model,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     50,
				"completion_tokens": 20,
				"total_tokens":      70,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

// MockAnthropicServer returns a test server that mimics Anthropic's API
func MockAnthropicServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req MockLLMRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"id":          "mock-completion-id",
			"type":        "completion",
			"model":       req.Model,
			"completion":  "Mock response for: " + req.Model,
			"stop_reason": "stop",
			"usage": map[string]interface{}{
				"input_tokens":  50,
				"output_tokens": 20,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}
