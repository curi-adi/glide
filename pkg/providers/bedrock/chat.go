package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/EinStack/glide/pkg/api/schemas"

	"go.uber.org/zap"

	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is an Bedrock-specific request schema
type ChatRequest struct {
	Messages             string               `json:"inputText"`
	TextGenerationConfig TextGenerationConfig `json:"textGenerationConfig"`
}

type TextGenerationConfig struct {
	Temperature   float64  `json:"temperature"`
	TopP          float64  `json:"topP"`
	MaxTokenCount int      `json:"maxTokenCount"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

// NewChatRequestFromConfig fills the struct from the config. Not using reflection because of performance penalty it gives
func NewChatRequestFromConfig(cfg *Config) *ChatRequest {
	return &ChatRequest{
		TextGenerationConfig: TextGenerationConfig{
			MaxTokenCount: cfg.DefaultParams.MaxTokens,
			StopSequences: cfg.DefaultParams.StopSequence,
			Temperature:   cfg.DefaultParams.Temperature,
			TopP:          cfg.DefaultParams.TopP,
		},
	}
}

func NewChatMessagesFromUnifiedRequest(request *schemas.ChatRequest) string {
	// message history not yet supported for AWS models
	message := fmt.Sprintf("Role: %s, Content: %s", request.Message.Role, request.Message.Content)

	return message
}

// Chat sends a chat request to the specified bedrock model.
func (c *Client) Chat(ctx context.Context, request *schemas.ChatRequest) (*schemas.ChatResponse, error) {
	// Create a new chat request
	chatRequest := c.createChatRequestSchema(request)

	chatResponse, err := c.doChatRequest(ctx, chatRequest)
	if err != nil {
		return nil, err
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
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal chat request payload: %w", err)
	}

	result, err := c.bedrockClient.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.Model),
		ContentType: aws.String("application/json"),
		Body:        rawPayload,
	})
	if err != nil {
		c.telemetry.Logger.Error("Error: Couldn't invoke model. Here's why: %v\n", zap.Error(err))
		return nil, err
	}

	var bedrockCompletion ChatCompletion

	err = json.Unmarshal(result.Body, &bedrockCompletion)
	if err != nil {
		c.telemetry.Logger.Error("failed to parse bedrock chat response", zap.Error(err))

		return nil, err
	}

	response := schemas.ChatResponse{
		ID:        uuid.NewString(),
		Created:   int(time.Now().Unix()),
		Provider:  "aws-bedrock",
		ModelName: c.config.Model,
		Cached:    false,
		ModelResponse: schemas.ModelResponse{
			SystemID: map[string]string{
				"system_fingerprint": "none",
			},
			Message: schemas.ChatMessage{
				Role:    "assistant",
				Content: bedrockCompletion.Results[0].OutputText,
				Name:    "",
			},
			TokenUsage: schemas.TokenUsage{
				PromptTokens:   bedrockCompletion.Results[0].TokenCount,
				ResponseTokens: -1,
				TotalTokens:    bedrockCompletion.Results[0].TokenCount,
			},
		},
	}

	return &response, nil
}
