package sdk

import "context"

// AppContext is the extension-facing registration surface.
type AppContext interface {
	RegisterProvider(provider Provider)
	RegisterChannel(channel Channel)
	RegisterTool(tool Tool)
	RegisterHook(hook Hook)
}

// Plugin registers one or more capabilities into the application.
type Plugin interface {
	ID() string
	Register(ctx context.Context, app AppContext) error
}
