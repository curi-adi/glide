package anthropic

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/EinStack/glide/pkg/telemetry"

	"github.com/EinStack/glide/pkg/providers/clients"
)

const (
	providerName = "anthropic"
)

// ErrEmptyResponse is returned when the OpenAI API returns an empty response.
var (
	ErrEmptyResponse = errors.New("empty response")
)

// Client is a client for accessing OpenAI API
type Client struct {
	baseURL             string
	chatURL             string
	apiVersion          string
	chatRequestTemplate *ChatRequest
	errMapper           *ErrorMapper
	config              *Config
	httpClient          *http.Client
	tel                 *telemetry.Telemetry
}

// NewClient creates a new OpenAI client for the OpenAI API.
func NewClient(providerConfig *Config, clientConfig *clients.ClientConfig, tel *telemetry.Telemetry) (*Client, error) {
	chatURL, err := url.JoinPath(providerConfig.BaseURL, providerConfig.ChatEndpoint)
	if err != nil {
		return nil, err
	}

	c := &Client{
		baseURL:             providerConfig.BaseURL,
		chatURL:             chatURL,
		apiVersion:          providerConfig.APIVersion,
		config:              providerConfig,
		chatRequestTemplate: NewChatRequestFromConfig(providerConfig),
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
