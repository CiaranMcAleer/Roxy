package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr string `yaml:"listen_addr"`

	// API Keys configuration
	APIKeys []APIKeyConfig `yaml:"api_keys"`

	// Model substitution rules
	ModelRules []ModelRule `yaml:"model_rules"`

	// Provider configurations
	Providers ProviderConfig `yaml:"providers"`
}

type APIKeyConfig struct {
	Key         string `yaml:"key"`
	KeyEnvVar   string `yaml:"key_env_var"` // Environment variable name for the key
	Provider    string `yaml:"provider"`
	MaxRPM      int    `yaml:"max_rpm"`      // Requests per minute
	MaxTPM      int    `yaml:"max_tpm"`      // Tokens per minute
	CooldownSec int    `yaml:"cooldown_sec"` // Cooldown period in seconds
}

type ModelRule struct {
	SourceModel     string   `yaml:"source_model"`
	TargetModels    []string `yaml:"target_models"`
	SelectionPolicy string   `yaml:"selection_policy"` // random, roundrobin, fallback
}

type ProviderConfig struct {
	OpenAI struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"openai"`
	Anthropic struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"anthropic"`
	OpenRouter struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"openrouter"`
	Chutes struct {
		BaseURL string `yaml:"base_url"`
	} `yaml:"chutes"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.loadSecrets(); err != nil {
		return nil, fmt.Errorf("loading secrets: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) loadSecrets() error {
	for i, key := range c.APIKeys {
		// If KeyEnvVar is specified, use it to load the key
		if key.KeyEnvVar != "" {
			envKey := os.Getenv(key.KeyEnvVar)
			if envKey == "" {
				return fmt.Errorf("environment variable %s not set for API key %d", key.KeyEnvVar, i)
			}
			c.APIKeys[i].Key = envKey
		}
	}
	return nil
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}

	if len(c.APIKeys) == 0 {
		return fmt.Errorf("at least one API key is required")
	}

	for i, key := range c.APIKeys {
		if key.Key == "" && key.KeyEnvVar == "" {
			return fmt.Errorf("api_keys[%d]: either key or key_env_var is required", i)
		}
		if key.Provider == "" {
			return fmt.Errorf("api_keys[%d]: provider is required", i)
		}
		if key.MaxRPM <= 0 {
			return fmt.Errorf("api_keys[%d]: max_rpm must be positive", i)
		}
		if key.MaxTPM <= 0 {
			return fmt.Errorf("api_keys[%d]: max_tpm must be positive", i)
		}
		if !isValidProvider(key.Provider) {
			return fmt.Errorf("api_keys[%d]: invalid provider %s", i, key.Provider)
		}
	}

	for i, rule := range c.ModelRules {
		if rule.SourceModel == "" {
			return fmt.Errorf("model_rules[%d]: source_model is required", i)
		}
		if len(rule.TargetModels) == 0 {
			return fmt.Errorf("model_rules[%d]: at least one target_model is required", i)
		}
		if rule.SelectionPolicy == "" {
			return fmt.Errorf("model_rules[%d]: selection_policy is required", i)
		}
		if !isValidSelectionPolicy(rule.SelectionPolicy) {
			return fmt.Errorf("model_rules[%d]: invalid selection_policy: %s", i, rule.SelectionPolicy)
		}
	}

	return nil
}

func isValidProvider(provider string) bool {
	validProviders := map[string]bool{
		"openai":     true,
		"anthropic":  true,
		"openrouter": true,
		"chutes":     true,
	}
	return validProviders[strings.ToLower(provider)]
}

func isValidSelectionPolicy(policy string) bool {
	validPolicies := map[string]bool{
		"random":     true,
		"roundrobin": true,
		"fallback":   true,
	}
	return validPolicies[strings.ToLower(policy)]
}
