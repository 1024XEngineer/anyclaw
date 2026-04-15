package sdk

import "context"

// InboundMessage is the normalized inbound message shape emitted by channels.
type InboundMessage struct {
	Channel   string
	AccountID string
	PeerID    string
	PeerKind  string
	Text      string
	MessageID string
	Raw       map[string]any
}

// OutboundMessage is the normalized outbound message shape accepted by channels.
type OutboundMessage struct {
	Channel   string
	AccountID string
	TargetID  string
	Text      string
	ReplyTo   string
	Metadata  map[string]any
}

// Channel is a messaging adapter that can receive and send messages.
type Channel interface {
	ID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg OutboundMessage) error
	SetInboundHandler(func(context.Context, InboundMessage) error)
}

// ChannelRegistry stores available messaging channels.
type ChannelRegistry interface {
	Register(ch Channel)
	Get(id string) (Channel, bool)
	List() []Channel
}
