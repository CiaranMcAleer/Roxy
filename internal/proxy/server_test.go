package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourusername/roxy/internal/config"
	"github.com/yourusername/roxy/internal/testutils"
)

func TestProxyServer(t *testing.T) {
	// Start mock LLM provider servers
	mockOpenAI := testutils.MockOpenAIServer()
	defer mockOpenAI.Close()

	mockAnthropic := testutils.MockAnthropicServer()
	defer mockAnthropic.Close()

	// Create test configuration
	cfg := &config.Config{
		ListenAddr: ":8080",
		APIKeys: []config.APIKeyConfig{
			{
				Key:      "test-openai-key",
				Provider: "openai",
				MaxRPM:   60,
				MaxTPM:   40000,
			},
			{
				Key:      "test-anthropic-key",
				Provider: "anthropic",
				MaxRPM:   60,
				MaxTPM:   40000,
			},
		},
		ModelRules: []config.ModelRule{
			{
				SourceModel:     "gpt-4",
				TargetModels:    []string{"gpt-4", "claude-2"},
				SelectionPolicy: "fallback",
			},
		},
		Providers: config.ProviderConfig{
			OpenAI: struct {
				BaseURL string `yaml:"base_url"`
			}{
				BaseURL: mockOpenAI.URL,
			},
			Anthropic: struct {
				BaseURL string `yaml:"base_url"`
			}{
				BaseURL: mockAnthropic.URL,
			},
		},
	}

	// Create and start proxy server
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	testCases := []struct {
		name           string
		requestBody    testutils.MockLLMRequest
		expectedModel  string
		expectedStatus int
	}{
		{
			name: "direct gpt-4 request",
			requestBody: testutils.MockLLMRequest{
				Model: "gpt-4",
				Messages: []testutils.Message{
					{Role: "user", Content: "Hello"},
				},
			},
			expectedModel:  "gpt-4",
			expectedStatus: http.StatusOK,
		},
		{
			name: "fallback to claude-2",
			requestBody: testutils.MockLLMRequest{
				Model: "gpt-4",
				Messages: []testutils.Message{
					{Role: "user", Content: "Hello"},
				},
			},
			expectedModel:  "claude-2",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.requestBody)
			if err != nil {
				t.Fatalf("Failed to marshal request body: %v", err)
			}

			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			server.handleProxy(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			var response map[string]interface{}
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if model, ok := response["model"].(string); !ok || model != tc.expectedModel {
				t.Errorf("Expected model %s, got %s", tc.expectedModel, model)
			}
		})
	}
}
