package rotation

import (
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/roxy/internal/config"
)

type KeyRotator struct {
	keys     []*ApiKey
	mu       sync.RWMutex
	lastUsed map[string]time.Time
}

type ApiKey struct {
	Config     config.APIKeyConfig
	usageCount int
	lastUsed   time.Time
}

func NewKeyRotator(configs []config.APIKeyConfig) *KeyRotator {
	keys := make([]*ApiKey, len(configs))
	for i, cfg := range configs {
		keys[i] = &ApiKey{
			Config:     cfg,
			usageCount: 0,
			lastUsed:   time.Time{},
		}
	}

	return &KeyRotator{
		keys:     keys,
		lastUsed: make(map[string]time.Time),
	}
}

func (kr *KeyRotator) GetKey(provider string) (*ApiKey, error) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	now := time.Now()
	for _, key := range kr.keys {
		if key.Config.Provider != provider {
			continue
		}

		// Check if key is within rate limits
		if now.Sub(key.lastUsed) >= time.Minute {
			key.usageCount = 0 // Reset counter after a minute
		}

		if key.usageCount < key.Config.MaxRPM {
			return key, nil
		}
	}

	return nil, fmt.Errorf("no available keys for provider: %s", provider)
}

func (kr *KeyRotator) ReportUsage(key *ApiKey, tokens int) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	key.usageCount++
	key.lastUsed = time.Now()
}
