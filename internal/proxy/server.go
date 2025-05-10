package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/roxy/internal/config"
	"github.com/yourusername/roxy/internal/rotation"
)

type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	rotator    *rotation.KeyRotator
	mu         sync.RWMutex
	// Add round-robin counters
	modelCounters map[string]int
}

type LLMRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages,omitempty"`
	Prompt      string        `json:"prompt,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewServer(cfg *config.Config) (*Server, error) {
	rotator := rotation.NewKeyRotator(cfg.APIKeys)

	server := &Server{
		cfg:           cfg,
		rotator:       rotator,
		modelCounters: make(map[string]int),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/", server.handleProxy)

	server.httpServer = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	return server, nil
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown() error {
	return s.httpServer.Shutdown(context.Background())
}

func (s *Server) getNextModelIndex(model string, total int) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.modelCounters[model]
	next := (current + 1) % total
	s.modelCounters[model] = next
	return next
}

func (s *Server) getTargetModel(sourceModel string) (string, string) {
	for _, rule := range s.cfg.ModelRules {
		if rule.SourceModel == sourceModel {
			switch rule.SelectionPolicy {
			case "random":
				model := rule.TargetModels[rand.Intn(len(rule.TargetModels))]
				return model, getProviderForModel(model)
			case "roundrobin":
				idx := s.getNextModelIndex(sourceModel, len(rule.TargetModels))
				model := rule.TargetModels[idx]
				return model, getProviderForModel(model)
			case "fallback":
				for _, model := range rule.TargetModels {
					provider := getProviderForModel(model)
					if key, err := s.rotator.GetKey(provider); err == nil {
						s.rotator.ReportUsage(key, 0) // Return key to pool
						return model, provider
					}
				}
			}
		}
	}
	return sourceModel, getProviderForModel(sourceModel)
}

func getProviderForModel(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-"):
		return "openai"
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "openrouter/"):
		return "openrouter"
	case strings.HasPrefix(model, "chutes/"):
		return "chutes"
	default:
		return "openai" // Default to OpenAI
	}
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Parse the request
	var req LLMRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Get target model and provider
	targetModel, provider := s.getTargetModel(req.Model)

	// Get API key
	key, err := s.rotator.GetKey(provider)
	if err != nil {
		http.Error(w, "No available API keys", http.StatusTooManyRequests)
		return
	}
	defer s.rotator.ReportUsage(key, 0) // Will be updated with actual token count

	// Modify request for target model
	req.Model = targetModel
	modifiedBody, err := json.Marshal(req)
	if err != nil {
		http.Error(w, "Failed to prepare request", http.StatusInternalServerError)
		return
	}

	// Prepare the provider request
	var targetURL string
	switch provider {
	case "openai":
		targetURL = fmt.Sprintf("%s/chat/completions", s.cfg.Providers.OpenAI.BaseURL)
	case "anthropic":
		targetURL = fmt.Sprintf("%s/complete", s.cfg.Providers.Anthropic.BaseURL)
	default:
		http.Error(w, "Unsupported provider", http.StatusBadRequest)
		return
	}

	// Create provider request
	proxyReq, err := http.NewRequest(r.Method, targetURL, bytes.NewBuffer(modifiedBody))
	if err != nil {
		http.Error(w, "Failed to create provider request", http.StatusInternalServerError)
		return
	}

	// Copy headers and set authentication
	copyHeaders(proxyReq.Header, r.Header)
	switch provider {
	case "openai":
		proxyReq.Header.Set("Authorization", "Bearer "+key.Config.Key)
	case "anthropic":
		proxyReq.Header.Set("X-Api-Key", key.Config.Key)
	}

	// Make the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Provider request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// If we get a rate limit error and we're using the first model,
	// try the next model in the fallback chain
	if resp.StatusCode == http.StatusTooManyRequests {
		for _, rule := range s.cfg.ModelRules {
			if rule.SourceModel == req.Model && rule.SelectionPolicy == "fallback" {
				// Try the next model in the chain
				for i := 1; i < len(rule.TargetModels); i++ {
					nextModel := rule.TargetModels[i]
					nextProvider := getProviderForModel(nextModel)
					nextKey, err := s.rotator.GetKey(nextProvider)
					if err != nil {
						continue
					}

					req.Model = nextModel
					modifiedBody, err = json.Marshal(req)
					if err != nil {
						http.Error(w, "Failed to prepare request", http.StatusInternalServerError)
						return
					}

					var targetURL string
					switch nextProvider {
					case "openai":
						targetURL = fmt.Sprintf("%s/chat/completions", s.cfg.Providers.OpenAI.BaseURL)
					case "anthropic":
						targetURL = fmt.Sprintf("%s/complete", s.cfg.Providers.Anthropic.BaseURL)
					}

					proxyReq, err = http.NewRequest(r.Method, targetURL, bytes.NewBuffer(modifiedBody))
					if err != nil {
						continue
					}

					copyHeaders(proxyReq.Header, r.Header)
					switch nextProvider {
					case "openai":
						proxyReq.Header.Set("Authorization", "Bearer "+nextKey.Config.Key)
					case "anthropic":
						proxyReq.Header.Set("X-Api-Key", nextKey.Config.Key)
					}

					resp, err = client.Do(proxyReq)
					if err == nil && resp.StatusCode == http.StatusOK {
						break
					}
					if resp != nil {
						resp.Body.Close()
					}
				}
				break
			}
		}
	}

	// Copy the final response
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
