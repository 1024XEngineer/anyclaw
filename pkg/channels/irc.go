package channel

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
)

type IRCChannelConfig struct {
	Enabled  bool     `json:"enabled"`
	Server   string   `json:"server"`
	Port     int      `json:"port"`
	Nick     string   `json:"nick"`
	Password string   `json:"password,omitempty"`
	Channels []string `json:"channels"`
	UseTLS   bool     `json:"use_tls"`
	RealName string   `json:"real_name,omitempty"`
}

type IRCAdapter struct {
	BaseAdapter
	config   IRCChannelConfig
	conn     net.Conn
	mu       sync.RWMutex
	handler  InboundHandler
	channels map[string]bool
}

func NewIRCAdapter(cfg IRCChannelConfig) *IRCAdapter {
	return &IRCAdapter{
		BaseAdapter: NewBaseAdapter("irc", cfg.Enabled),
		config:      cfg,
		channels:    make(map[string]bool),
	}
}

func (a *IRCAdapter) Enabled() bool {
	return a.config.Enabled
}

func (a *IRCAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if !a.config.Enabled {
		return nil
	}
	a.handler = handle
	a.setRunning(true)
	defer a.setRunning(false)

	addr := net.JoinHostPort(a.config.Server, fmt.Sprintf("%d", a.config.Port))
	var conn net.Conn
	var err error

	if a.config.UseTLS {
		conn, err = tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		a.setError(fmt.Errorf("failed to connect to IRC server: %w", err))
		return err
	}
	a.conn = conn
	defer conn.Close()

	// Send registration
	nick := a.config.Nick
	if nick == "" {
		nick = "anyclaw"
	}
	realName := a.config.RealName
	if realName == "" {
		realName = "AnyClaw Bot"
	}

	fmt.Fprintf(conn, "NICK %s\r\n", nick)
	fmt.Fprintf(conn, "USER %s 0 * :%s\r\n", nick, realName)

	if a.config.Password != "" {
		fmt.Fprintf(conn, "PASS %s\r\n", a.config.Password)
	}

	// Join channels
	for _, ch := range a.config.Channels {
		fmt.Fprintf(conn, "JOIN %s\r\n", ch)
		a.channels[ch] = true
	}

	a.setError(nil)
	a.markActivity()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		a.handleIRCMessage(ctx, conn, line)
		a.markActivity()
	}

	return scanner.Err()
}

func (a *IRCAdapter) handleIRCMessage(ctx context.Context, conn net.Conn, line string) {
	// Handle PING
	if strings.HasPrefix(line, "PING") {
		fmt.Fprintf(conn, "PONG %s\r\n", strings.TrimPrefix(line, "PING "))
		return
	}

	// Parse IRC message
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 {
		return
	}

	source := parts[0]
	command := strings.ToUpper(parts[1])
	target := parts[2]
	text := strings.TrimPrefix(parts[3], ":")

	// Handle PRIVMSG
	if command == "PRIVMSG" {
		sender := ""
		if idx := strings.Index(source, "!"); idx > 0 {
			sender = source[1:idx]
		}

		// Check if it's a channel message or PM
		sessionID := target
		if !strings.HasPrefix(target, "#") {
			sessionID = sender
		}

		if a.handler != nil {
			meta := map[string]string{
				"channel":  "irc",
				"user_id":  sender,
				"username": sender,
				"chat_id":  target,
			}
			response, _, err := a.handler(ctx, sessionID, text, meta)
			if err == nil && response != "" {
				fmt.Fprintf(conn, "PRIVMSG %s :%s\r\n", target, response)
			}
		}
	}
}
