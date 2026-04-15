package webchat

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Channel is the starter WebChat channel adapter.
type Channel struct {
	inboundHandler func(context.Context, sdk.InboundMessage) error
}

// New returns a new WebChat channel.
func New() *Channel { return &Channel{} }

// ID returns the channel identifier.
func (c *Channel) ID() string { return "webchat" }

// Start starts the channel runtime.
func (c *Channel) Start(context.Context) error { return nil }

// Stop stops the channel runtime.
func (c *Channel) Stop(context.Context) error { return nil }

// Send sends an outbound message to the channel.
func (c *Channel) Send(context.Context, sdk.OutboundMessage) error { return nil }

// SetInboundHandler installs the inbound delivery callback.
func (c *Channel) SetInboundHandler(handler func(context.Context, sdk.InboundMessage) error) {
	c.inboundHandler = handler
}
