package azureopenai

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/EinStack/glide/pkg/providers/openai"

	"github.com/EinStack/glide/pkg/telemetry"

	"github.com/EinStack/glide/pkg/providers/clients"
)

const (
	providerName = "azureopenai"
)

// ErrEmptyResponse is returned when the OpenAI API returns an empty response.
var (
	ErrEmptyResponse = errors.New("empty response")
)

// Client is a client for accessing Azure OpenAI API
type Client struct {
	baseURL             string // The name of your Azure OpenAI Resource (e.g https://glide-test.openai.azure.com/)
	chatURL             string
	chatRequestTemplate *ChatRequest
	finishReasonMapper  *openai.FinishReasonMapper
	errMapper           *ErrorMapper
	config              *Config
	httpClient          *http.Client
	tel                 *telemetry.Telemetry
}

// NewClient creates a new Azure OpenAI client for the OpenAI API.
func NewClient(providerConfig *Config, clientConfig *clients.ClientConfig, tel *telemetry.Telemetry) (*Client, error) {
	chatURL := fmt.Sprintf(
		"%s/openai/deployments/%s/chat/completions?api-version=%s",
		providerConfig.BaseURL,
		providerConfig.Model,
		providerConfig.APIVersion,
	)

	c := &Client{
		baseURL:             providerConfig.BaseURL,
		chatURL:             chatURL,
		config:              providerConfig,
		chatRequestTemplate: NewChatRequestFromConfig(providerConfig),
		finishReasonMapper:  openai.NewFinishReasonMapper(tel),
		errMapper:           NewErrorMapper(tel),
		httpClient: &http.Client{
			Timeout: *clientConfig.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        *clientConfig.MaxIdleConns,
				MaxIdleConnsPerHost: *clientConfig.MaxIdleConnsPerHost,
			},
		},
		tel: tel,
	}

	return c, nil
}

func (c *Client) Provider() string {
	return providerName
}
