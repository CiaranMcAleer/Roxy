package rotation

import (
	"testing"
	"time"

	"github.com/CiaranMcAleer/roxy/internal/config"
)

func TestKeyRotator(t *testing.T) {
	testCases := []struct {
		name     string
		configs  []config.APIKeyConfig
		provider string
		wantErr  bool
	}{
		{
			name: "single key rotation",
			configs: []config.APIKeyConfig{
				{
					Key:      "test-key-1",
					Provider: "openai",
					MaxRPM:   60,
					MaxTPM:   40000,
				},
			},
			provider: "openai",
			wantErr:  false,
		},
		{
			name: "multiple keys rotation",
			configs: []config.APIKeyConfig{
				{
					Key:      "test-key-1",
					Provider: "openai",
					MaxRPM:   60,
					MaxTPM:   40000,
				},
				{
					Key:      "test-key-2",
					Provider: "openai",
					MaxRPM:   60,
					MaxTPM:   40000,
				},
			},
			provider: "openai",
			wantErr:  false,
		},
		{
			name: "provider not found",
			configs: []config.APIKeyConfig{
				{
					Key:      "test-key-1",
					Provider: "openai",
					MaxRPM:   60,
					MaxTPM:   40000,
				},
			},
			provider: "anthropic",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rotator := NewKeyRotator(tc.configs)

			// Test multiple rotations
			for i := 0; i < 10; i++ {
				key, err := rotator.GetKey(tc.provider)
				if tc.wantErr {
					if err == nil {
						t.Errorf("Expected error but got none")
					}
					return
				}

				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if key == nil {
					t.Fatal("Expected key but got nil")
				}

				if key.Config.Provider != tc.provider {
					t.Errorf("Got provider %s, want %s", key.Config.Provider, tc.provider)
				}

				// Report some usage
				rotator.ReportUsage(key, 100)
			}
		})
	}
}

func TestRateLimiting(t *testing.T) {
	configs := []config.APIKeyConfig{
		{
			Key:      "test-key-1",
			Provider: "openai",
			MaxRPM:   2,
			MaxTPM:   1000,
		},
	}

	rotator := NewKeyRotator(configs)

	// Make initial requests
	key1, err := rotator.GetKey("openai")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	rotator.ReportUsage(key1, 100)

	key2, err := rotator.GetKey("openai")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	rotator.ReportUsage(key2, 100)

	// Third request should be rate limited
	_, err = rotator.GetKey("openai")
	if err == nil {
		t.Error("Expected rate limit error but got none")
	}

	// Wait for rate limit to reset
	time.Sleep(time.Minute)

	// Should be able to get a key again
	key3, err := rotator.GetKey("openai")
	if err != nil {
		t.Fatalf("Unexpected error after rate limit reset: %v", err)
	}
	if key3 == nil {
		t.Fatal("Expected key but got nil after rate limit reset")
	}
}
