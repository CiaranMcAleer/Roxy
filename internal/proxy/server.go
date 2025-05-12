package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CiaranMcAleer/roxy/internal/config"
	"github.com/CiaranMcAleer/roxy/internal/rotation"
)

type Server struct {
	cfg        *config.Config
	httpServer *http.Server
	rotator    *rotation.KeyRotator
	mu         sync.RWMutex
	// Add round-robin counters
	modelCounters map[string]int
	commandHandler *CommandHandler
	cache      *cache.Cache
}

type CommandHandler struct {
	cfg     *config.Config
	rotator *rotation.KeyRotator
	mu      sync.RWMutex
}

func NewCommandHandler(cfg *config.Config, rotator *rotation.KeyRotator) *CommandHandler {
	return &CommandHandler{
		cfg:     cfg,
		rotator: rotator,
	}
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
		commandHandler: NewCommandHandler(cfg, rotator),
		cache:        cache.New(5 * time.Minute), // 5 minute TTL
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

	// Check for #roxy commands
	if strings.HasPrefix(string(body), "#roxy") {
		s.handleCommand(w, body)
		return
	}

	// Parse the request
	var req LLMRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Check cache
	cacheKey := generateCacheKey(&req)
	if cached, exists := s.cache.Get(cacheKey); exists {
		w.Write(cached)
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

	// Cache and return the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	if resp.StatusCode == http.StatusOK {
		s.cache.Set(cacheKey, respBody)
	}

	// Copy the final response
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func generateCacheKey(req *LLMRequest) string {
	hash := sha256.New()
	hash.Write([]byte(req.Model))
	for _, msg := range req.Messages {
		hash.Write([]byte(msg.Role))
		hash.Write([]byte(msg.Content))
	}
	hash.Write([]byte(fmt.Sprintf("%d", req.MaxTokens)))
	hash.Write([]byte(fmt.Sprintf("%f", req.Temperature)))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func (s *Server) handleCommand(w http.ResponseWriter, body []byte) {
	cmd := strings.TrimSpace(string(body))
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		http.Error(w, "Invalid command format", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "add":
		s.commandHandler.handleAddCommand(w, parts[2:])
	case "list":
		s.commandHandler.handleListCommand(w, parts[2:])
	case "help":
		s.commandHandler.handleHelpCommand(w)
	default:
		http.Error(w, "Unknown command", http.StatusBadRequest)
	}
}

func (h *CommandHandler) handleAddCommand(w http.ResponseWriter, args []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(args) < 3 || args[0] != "key" {
		http.Error(w, "Usage: #roxy add key [provider] [key]", http.StatusBadRequest)
		return
	}

	provider := args[1]
	key := args[2]

	newKey := config.APIKeyConfig{
		Provider: provider,
		Key:      key,
		MaxRPM:   1000, // Default values
		MaxTPM:   100000,
	}

	h.cfg.APIKeys = append(h.cfg.APIKeys, newKey)
	h.rotator.AddKey(newKey)

	fmt.Fprintf(w, "Added key for provider: %s", provider)
}

func (h *CommandHandler) handleListCommand(w http.ResponseWriter, args []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(args) < 1 || args[0] != "keys" {
		http.Error(w, "Usage: #roxy list keys", http.StatusBadRequest)
		return
	}

	for _, key := range h.cfg.APIKeys {
		fmt.Fprintf(w, "Provider: %s, Key: %s...\n", key.Provider, key.Key[:4])
	}
}

func (h *CommandHandler) handleHelpCommand(w http.ResponseWriter) {
	helpText := `Available commands:
#roxy add key [provider] [key] - Add new API key
#roxy list keys - List configured API keys
#roxy help - Show this help message`
	
	fmt.Fprint(w, helpText)
}
