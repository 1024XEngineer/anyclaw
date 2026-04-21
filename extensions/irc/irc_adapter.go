package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

type Config struct {
	Server   string   `json:"server"`
	Port     int      `json:"port"`
	Nick     string   `json:"nick"`
	Channels []string `json:"channels"`
	UseTLS   bool     `json:"use_tls"`
}

type IRCExtension struct {
	config Config
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func NewIRCExtension(cfg Config) *IRCExtension {
	if cfg.Server == "" {
		cfg.Server = "irc.libera.chat"
	}
	if cfg.Port == 0 {
		cfg.Port = 6667
	}
	return &IRCExtension{
		config: cfg,
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func (e *IRCExtension) Run(ctx context.Context) error {
	if strings.TrimSpace(e.config.Nick) == "" {
		return fmt.Errorf("nick is required")
	}
	if len(e.config.Channels) == 0 {
		return fmt.Errorf("channels are required")
	}
	if err := e.connect(); err != nil {
		return err
	}
	defer e.conn.Close()

	for _, channel := range e.config.Channels {
		if err := e.sendRaw("JOIN " + channel); err != nil {
			return err
		}
	}

	lines := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		for {
			line, err := e.reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			lines <- strings.TrimSpace(line)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			_ = e.sendRaw("QUIT :shutdown")
			return nil
		case err := <-errCh:
			return err
		case line := <-lines:
			if err := e.handleLine(ctx, line); err != nil {
				return err
			}
		}
	}
}

func (e *IRCExtension) connect() error {
	address := fmt.Sprintf("%s:%d", e.config.Server, e.config.Port)
	var conn net.Conn
	var err error
	if e.config.UseTLS {
		conn, err = tls.Dial("tcp", address, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = net.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	e.conn = conn
	e.reader = bufio.NewReader(conn)
	e.writer = bufio.NewWriter(conn)

	if err := e.sendRaw("NICK " + e.config.Nick); err != nil {
		return err
	}
	if err := e.sendRaw(fmt.Sprintf("USER %s 0 * :%s", e.config.Nick, e.config.Nick)); err != nil {
		return err
	}
	return nil
}

func (e *IRCExtension) handleLine(ctx context.Context, line string) error {
	if strings.HasPrefix(line, "PING :") {
		return e.sendRaw("PONG :" + strings.TrimPrefix(line, "PING :"))
	}

	channel, text, user, ok := parsePrivmsg(line)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}

	payload := map[string]any{
		"action":  "message",
		"channel": "irc",
		"chat_id": channel,
		"text":    text,
		"user_id": user,
	}
	if err := json.NewEncoder(e.stdout).Encode(payload); err != nil {
		return err
	}

	reply, err := e.readReply()
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if reply != "" {
		return e.sendPrivmsg(channel, reply)
	}
	return nil
}

func parsePrivmsg(line string) (channel, text, user string, ok bool) {
	parts := strings.SplitN(line, " PRIVMSG ", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	prefix := strings.TrimPrefix(parts[0], ":")
	if bang := strings.Index(prefix, "!"); bang >= 0 {
		user = prefix[:bang]
	} else {
		user = prefix
	}

	messageParts := strings.SplitN(parts[1], " :", 2)
	if len(messageParts) != 2 {
		return "", "", "", false
	}

	channel = strings.TrimSpace(messageParts[0])
	text = messageParts[1]
	return channel, text, user, true
}

func (e *IRCExtension) readReply() (string, error) {
	var response map[string]any
	if err := json.NewDecoder(e.stdin).Decode(&response); err != nil {
		return "", err
	}
	reply, _ := response["text"].(string)
	return strings.TrimSpace(reply), nil
}

func (e *IRCExtension) sendPrivmsg(channel, text string) error {
	return e.sendRaw(fmt.Sprintf("PRIVMSG %s :%s", channel, text))
}

func (e *IRCExtension) sendRaw(line string) error {
	if e.writer == nil {
		return fmt.Errorf("irc connection not established")
	}
	if _, err := e.writer.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return e.writer.Flush()
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

	ext := NewIRCExtension(cfg)
	if err := ext.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
