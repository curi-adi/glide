package bedrock

import (
	"context"

	"github.com/EinStack/glide/pkg/providers/clients"

	"github.com/EinStack/glide/pkg/api/schemas"
)

func (c *Client) SupportChatStream() bool {
	return false
}

func (c *Client) ChatStream(_ context.Context, _ *schemas.ChatStreamRequest) (clients.ChatStream, error) {
	return nil, clients.ErrChatStreamNotImplemented
}
