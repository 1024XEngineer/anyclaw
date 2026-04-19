package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Channel      string `json:"channel"`
	OAuthToken   string `json:"oauth_token"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

type TwitchAdapter struct {
	config Config
	conn   net.Conn
	reader *bufio.Reader
}

func NewTwitchAdapter(cfg Config) *TwitchAdapter {
	return &TwitchAdapter{config: cfg}
}

func (a *TwitchAdapter) connect(ctx context.Context) error {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", "irc.chat.twitch.tv:6667")
	if err != nil {
		return fmt.Errorf("failed to connect to twitch irc: %w", err)
	}
	a.conn = conn
	a.reader = bufio.NewReader(conn)
	return nil
}

func (a *TwitchAdapter) sendLine(line string) error {
	_, err := fmt.Fprint(a.conn, line+"\r\n")
	return err
}

func (a *TwitchAdapter) authenticate() error {
	token := a.config.OAuthToken
	if !strings.HasPrefix(token, "oauth:") {
		token = "oauth:" + token
	}
	if err := a.sendLine("PASS " + token); err != nil {
		return err
	}
	nick := strings.ToLower(a.config.Channel)
	if err := a.sendLine("NICK " + nick); err != nil {
		return err
	}
	return nil
}

func (a *TwitchAdapter) joinChannel() error {
	channel := strings.ToLower(a.config.Channel)
	if !strings.HasPrefix(channel, "#") {
		channel = "#" + channel
	}
	return a.sendLine("JOIN " + channel)
}

var privmsgRegex = regexp.MustCompile(`^:(\w+)!\w+@\w+\.tmi\.twitch\.tv PRIVMSG #(\w+) :(.+)$`)
var tagsRegex = regexp.MustCompile(`^@(.+) :(\w+)!\w+@\w+\.tmi\.twitch\.tv PRIVMSG #(\w+) :(.+)$`)

func parseMessage(line string) (displayName, username, channel, text string, ok bool) {
	if matches := tagsRegex.FindStringSubmatch(line); matches != nil {
		tags := matches[1]
		username = strings.ToLower(matches[2])
		channel = matches[3]
		text = matches[4]

		displayName = username
		if dm := regexp.MustCompile(`display-name=([^;]*)`).FindStringSubmatch(tags); dm != nil {
			if dm[1] != "" {
				displayName = dm[1]
			}
		}
		return displayName, username, channel, text, true
	}

	if matches := privmsgRegex.FindStringSubmatch(line); matches != nil {
		username = strings.ToLower(matches[1])
		channel = matches[2]
		text = matches[3]
		return username, username, channel, text, true
	}

	return "", "", "", "", false
}

func (a *TwitchAdapter) handlePing(line string) bool {
	if strings.HasPrefix(line, "PING") {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			a.sendLine("PONG " + parts[1])
		} else {
			a.sendLine("PONG")
		}
		return true
	}
	return false
}

func (a *TwitchAdapter) sendMessage(channel, text string) error {
	channelName := channel
	if strings.HasPrefix(channelName, "#") {
		channelName = channelName[1:]
	}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := a.sendLine("PRIVMSG #" + strings.ToLower(channelName) + " :" + line); err != nil {
			return err
		}
	}
	return nil
}

func (a *TwitchAdapter) Run(ctx context.Context) error {
	if err := a.connect(ctx); err != nil {
		return err
	}
	defer a.conn.Close()

	if err := a.authenticate(); err != nil {
		return err
	}

	if err := a.joinChannel(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "twitch: connected and joined #%s\n", a.config.Channel)

	go func() {
		<-ctx.Done()
		a.conn.Close()
	}()

	scanner := bufio.NewScanner(a.reader)
	scanner.Buffer(make([]byte, 4096), 65536)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			continue
		}

		if a.handlePing(line) {
			continue
		}

		displayName, username, channel, text, ok := parseMessage(line)
		if !ok {
			continue
		}

		input := map[string]any{
			"action":       "message",
			"channel":      "twitch",
			"chat_id":      channel,
			"text":         text,
			"user_id":      username,
			"display_name": displayName,
		}
		data, err := json.Marshal(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "twitch: marshal error: %v\n", err)
			continue
		}
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			fmt.Fprintf(os.Stderr, "twitch: read response error: %v\n", err)
			continue
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := a.sendMessage(channel, reply); err != nil {
				fmt.Fprintf(os.Stderr, "twitch: send reply error: %v\n", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("twitch: scanner error: %w", err)
	}

	return nil
}

func main() {
	configJSON := os.Getenv("ANYCLAW_EXTENSION_CONFIG")
	if configJSON == "" {
		fmt.Fprintln(os.Stderr, "missing ANYCLAW_EXTENSION_CONFIG")
		os.Exit(1)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Channel == "" || cfg.OAuthToken == "" {
		fmt.Fprintln(os.Stderr, "channel and oauth_token are required")
		os.Exit(1)
	}

	adapter := NewTwitchAdapter(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := adapter.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
