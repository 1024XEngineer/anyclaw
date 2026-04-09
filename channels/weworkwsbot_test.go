package channels

import (
	"context"
	"fmt"
	"testing"
	"time"

	"anyclaw/bus"
	"anyclaw/config"
	"github.com/google/uuid"
)

func TestNewWeWorkWsBotChannel(t *testing.T) {
	mgr := bus.NewMessageBus(16)
	cfg := config.WeWorkWsBotChannelConfig{
		Enabled:        true,
		BotID:          "aibAnLYMK4TgdOkkhCCYZabAtOGE64ouXFV",
		SecretID:       "krmWVv9crsg52IkvabpS0h8NQzV1X6Yk0yNIsiyeObX",
		URL:            "wss://openws.work.weixin.qq.com",
		Header:         nil,
		Reconnect:      false,
		ReconnectDelay: 3,
		Heartbeat:      30,
	}
	ctx, cancel := context.WithCancel(context.Background())
	channel, err := NewWeWorkWsBotChannel(cfg, mgr)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			imsg, err := mgr.ConsumeInbound(ctx)
			if err != nil {
				t.Error(err)
				break
			}
			t.Log(imsg)
			stream := make(chan *bus.StreamMessage, 10)
			go func() {
				time.Sleep(1 * time.Second)
				for i := 0; i < 20; i++ {
					time.Sleep(3 * time.Second)
					req := &bus.StreamMessage{
						ID:         uuid.New().String(),
						Channel:    "",
						ChatID:     imsg.ChatID,
						Content:    fmt.Sprintf("%d", i),
						ChunkIndex: 0,
						IsComplete: false,
						IsThinking: false,
						IsFinal:    false,
						Metadata:   nil,
						Error:      "",
					}
					stream <- req
				}
			}()
			channel.SendStream(imsg.ChatID, stream)
			//channel.Send(&resp)
		}
	}()
	channel.Start(ctx)
	time.Sleep(10 * time.Minute)
	cancel()
}
