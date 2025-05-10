package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoading(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `listen_addr: ":8080"
api_keys:
  - key: "test-openai-key"
    provider: "openai"
    max_rpm: 3500
    max_tpm: 90000
  - key: "test-anthropic-key"
    provider: "anthropic"
    max_rpm: 10000
    max_tpm: 100000

model_rules:
  - source_model: "gpt-4"
    target_models: 
      - "gpt-4"
      - "claude-2"
    selection_policy: "fallback"

providers:
  openai:
    base_url: "https://api.openai.com/v1"
  anthropic:
    base_url: "https://api.anthropic.com/v1"`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test configuration values
	if cfg.ListenAddr != ":8080" {
		t.Errorf("Expected listen_addr ':8080', got %s", cfg.ListenAddr)
	}

	if len(cfg.APIKeys) != 2 {
		t.Errorf("Expected 2 API keys, got %d", len(cfg.APIKeys))
	}

	// Test OpenAI key config
	openAIKey := cfg.APIKeys[0]
	if openAIKey.Key != "test-openai-key" || openAIKey.Provider != "openai" {
		t.Errorf("Unexpected OpenAI key config: %+v", openAIKey)
	}

	// Test Anthropic key config
	anthropicKey := cfg.APIKeys[1]
	if anthropicKey.Key != "test-anthropic-key" || anthropicKey.Provider != "anthropic" {
		t.Errorf("Unexpected Anthropic key config: %+v", anthropicKey)
	}

	// Test model rules
	if len(cfg.ModelRules) != 1 {
		t.Errorf("Expected 1 model rule, got %d", len(cfg.ModelRules))
	}

	rule := cfg.ModelRules[0]
	if rule.SourceModel != "gpt-4" || len(rule.TargetModels) != 2 {
		t.Errorf("Unexpected model rule: %+v", rule)
	}

	// Test provider configs
	if cfg.Providers.OpenAI.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("Unexpected OpenAI base URL: %s", cfg.Providers.OpenAI.BaseURL)
	}

	if cfg.Providers.Anthropic.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("Unexpected Anthropic base URL: %s", cfg.Providers.Anthropic.BaseURL)
	}
}

func TestConfigValidation(t *testing.T) {
	testCases := []struct {
		name        string
		config      string
		expectedErr bool
	}{
		{
			name: "missing listen address",
			config: `api_keys:
  - key: "test-key"
    provider: "openai"
    max_rpm: 3500
    max_tpm: 90000`,
			expectedErr: true,
		},
		{
			name: "missing api keys",
			config: `listen_addr: ":8080"
providers:
  openai:
    base_url: "https://api.openai.com/v1"`,
			expectedErr: true,
		},
		{
			name: "invalid yaml",
			config: `listen_addr: ":8080"
api_keys:
  - key: test-key
    provider: openai
    max_rpm: invalid
    max_tpm: 90000`,
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(configPath, []byte(tc.config), 0644); err != nil {
				t.Fatalf("Failed to create test config file: %v", err)
			}

			_, err := Load(configPath)
			if tc.expectedErr && err == nil {
				t.Error("Expected error but got none")
			} else if !tc.expectedErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
