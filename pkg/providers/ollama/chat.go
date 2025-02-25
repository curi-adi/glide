package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/EinStack/glide/pkg/providers/clients"

	"github.com/google/uuid"

	"github.com/EinStack/glide/pkg/api/schemas"

	"go.uber.org/zap"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is an ollama-specific request schema
type ChatRequest struct {
	Model        string        `json:"model"`
	Messages     []ChatMessage `json:"messages"`
	Microstat    int           `json:"microstat,omitempty"`
	MicrostatEta float64       `json:"microstat_eta,omitempty"`
	MicrostatTau float64       `json:"microstat_tau,omitempty"`
	NumCtx       int           `json:"num_ctx,omitempty"`
	NumGqa       int           `json:"num_gqa,omitempty"`
	NumGpu       int           `json:"num_gpu,omitempty"`
	NumThread    int           `json:"num_thread,omitempty"`
	RepeatLastN  int           `json:"repeat_last_n,omitempty"`
	Temperature  float64       `json:"temperature,omitempty"`
	Seed         int           `json:"seed,omitempty"`
	StopWords    []string      `json:"stop,omitempty"`
	Tfsz         float64       `json:"tfs_z,omitempty"`
	NumPredict   int           `json:"num_predict,omitempty"`
	TopK         int           `json:"top_k,omitempty"`
	TopP         float64       `json:"top_p,omitempty"`
	Stream       bool          `json:"stream"`
}

// NewChatRequestFromConfig fills the struct from the config. Not using reflection because of performance penalty it gives
func NewChatRequestFromConfig(cfg *Config) *ChatRequest {
	return &ChatRequest{
		Model:        cfg.Model,
		Temperature:  cfg.DefaultParams.Temperature,
		Microstat:    cfg.DefaultParams.Microstat,
		MicrostatEta: cfg.DefaultParams.MicrostatEta,
		MicrostatTau: cfg.DefaultParams.MicrostatTau,
		NumCtx:       cfg.DefaultParams.NumCtx,
		NumGqa:       cfg.DefaultParams.NumGqa,
		NumGpu:       cfg.DefaultParams.NumGpu,
		NumThread:    cfg.DefaultParams.NumThread,
		RepeatLastN:  cfg.DefaultParams.RepeatLastN,
		Seed:         cfg.DefaultParams.Seed,
		StopWords:    cfg.DefaultParams.StopWords,
		Tfsz:         cfg.DefaultParams.Tfsz,
		NumPredict:   cfg.DefaultParams.NumPredict,
		TopP:         cfg.DefaultParams.TopP,
		TopK:         cfg.DefaultParams.TopK,
		Stream:       cfg.DefaultParams.Stream,
	}
}

func NewChatMessagesFromUnifiedRequest(request *schemas.ChatRequest) []ChatMessage {
	messages := make([]ChatMessage, 0, len(request.MessageHistory)+1)

	// Add items from messageHistory first and the new chat message last
	for _, message := range request.MessageHistory {
		messages = append(messages, ChatMessage{Role: message.Role, Content: message.Content})
	}

	messages = append(messages, ChatMessage{Role: request.Message.Role, Content: request.Message.Content})

	return messages
}

// Chat sends a chat request to the specified ollama model.
func (c *Client) Chat(ctx context.Context, request *schemas.ChatRequest) (*schemas.ChatResponse, error) {
	// Create a new chat request
	chatRequest := c.createChatRequestSchema(request)

	chatResponse, err := c.doChatRequest(ctx, chatRequest)
	if err != nil {
		return nil, fmt.Errorf("chat request failed: %w", err)
	}

	if len(chatResponse.ModelResponse.Message.Content) == 0 {
		return nil, ErrEmptyResponse
	}

	return chatResponse, nil
}

func (c *Client) createChatRequestSchema(request *schemas.ChatRequest) *ChatRequest {
	// TODO: consider using objectpool to optimize memory allocation
	chatRequest := c.chatRequestTemplate // hoping to get a copy of the template
	chatRequest.Messages = NewChatMessagesFromUnifiedRequest(request)

	return chatRequest
}

func (c *Client) doChatRequest(ctx context.Context, payload *ChatRequest) (*schemas.ChatResponse, error) {
	// Build request payload
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal ollama chat request payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatURL, bytes.NewBuffer(rawPayload))
	if err != nil {
		return nil, fmt.Errorf("unable to create ollama chat request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// TODO: this could leak information from messages which may not be a desired thing to have
	c.telemetry.Logger.Debug(
		"ollama chat request",
		zap.String("chat_url", c.chatURL),
		zap.Any("payload", payload),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send ollama chat request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			c.telemetry.Logger.Error("failed to read ollama chat response", zap.Error(err))
		}

		c.telemetry.Logger.Error(
			"ollama chat request failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(bodyBytes)),
			zap.Any("headers", resp.Header),
		)

		if resp.StatusCode == http.StatusTooManyRequests {
			// Read the value of the "Retry-After" header to get the cooldown delay
			retryAfter := resp.Header.Get("Retry-After")

			// Parse the value to get the duration
			cooldownDelay, err := time.ParseDuration(retryAfter)
			if err != nil {
				return nil, fmt.Errorf("failed to parse cooldown delay from headers: %w", err)
			}

			return nil, clients.NewRateLimitError(&cooldownDelay)
		}

		// Server & client errors result in the same error to keep gateway resilient
		return nil, clients.ErrProviderUnavailable
	}

	// Read the response body into a byte slice
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.telemetry.Logger.Error("failed to read ollama chat response", zap.Error(err))
		return nil, err
	}

	// Parse the response JSON
	var ollamaCompletion ChatCompletion

	err = json.Unmarshal(bodyBytes, &ollamaCompletion)
	if err != nil {
		c.telemetry.Logger.Error("failed to parse ollama chat response", zap.Error(err))
		return nil, err
	}

	// Map response to UnifiedChatResponse schema
	response := schemas.ChatResponse{
		ID:        uuid.NewString(),
		Created:   int(time.Now().Unix()),
		Provider:  providerName,
		ModelName: ollamaCompletion.Model,
		Cached:    false,
		ModelResponse: schemas.ModelResponse{
			SystemID: map[string]string{
				"system_fingerprint": "",
			},
			Message: schemas.ChatMessage{
				Role:    ollamaCompletion.Message.Role,
				Content: ollamaCompletion.Message.Content,
			},
			TokenUsage: schemas.TokenUsage{
				PromptTokens:   ollamaCompletion.EvalCount,
				ResponseTokens: ollamaCompletion.EvalCount,
				TotalTokens:    ollamaCompletion.EvalCount,
			},
		},
	}

	return &response, nil
}
