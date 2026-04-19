package channels

import (
	"errors"
	"strings"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

type InboundHandler = inputlayer.InboundHandler
type StreamChunkHandler = inputlayer.StreamChunkHandler
type StreamAdapter = inputlayer.StreamAdapter
type Status = inputlayer.Status
type Adapter = inputlayer.Adapter
type BaseAdapter = inputlayer.BaseAdapter
type Manager = inputlayer.Manager

var NewBaseAdapter = inputlayer.NewBaseAdapter
var NewManager = inputlayer.NewManager

func streamWithMessageFallback(streamFn func(onChunk func(chunk string)) error, sendFinal func(text string) error) error {
	var accumulated strings.Builder
	err := streamFn(func(chunk string) {
		accumulated.WriteString(chunk)
	})

	final := accumulated.String()
	if err != nil {
		if strings.TrimSpace(final) != "" {
			if sendErr := sendFinal(final + "\n\n[Error: " + err.Error() + "]"); sendErr != nil {
				return errors.Join(err, sendErr)
			}
		}
		return err
	}
	if strings.TrimSpace(final) == "" {
		return nil
	}
	return sendFinal(final)
}
