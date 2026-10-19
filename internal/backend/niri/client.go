package niri

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/RandomLemon/midori/internal/display"
)

const (
	defaultDialTimeout   = 2 * time.Second
	defaultOpTimeout     = 5 * time.Second
	defaultMaxReplyBytes = 8 << 20
)

type Config struct {
	SocketPath    string
	DialTimeout   time.Duration
	OpTimeout     time.Duration
	MaxReplyBytes int
}

func DefaultConfig(socketPath string) Config {
	return Config{
		SocketPath:    socketPath,
		DialTimeout:   defaultDialTimeout,
		OpTimeout:     defaultOpTimeout,
		MaxReplyBytes: defaultMaxReplyBytes,
	}
}

type client struct {
	cfg Config
}

type requestError struct {
	message string
}

func (e *requestError) Error() string {
	return "niri: " + e.message
}

func (c *client) roundTrip(ctx context.Context, request []byte) (json.RawMessage, error) {
	if c.cfg.SocketPath == "" {
		return nil, fmt.Errorf("niri: NIRI_SOCKET is empty: %w", display.ErrConnection)
	}
	dialTimeout := c.cfg.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", c.cfg.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("niri: connect to %s: %w: %v", c.cfg.SocketPath, display.ErrConnection, err)
	}
	defer conn.Close()

	opTimeout := c.cfg.OpTimeout
	if opTimeout <= 0 {
		opTimeout = defaultOpTimeout
	}
	deadline := time.Now().Add(opTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("niri: set deadline: %w: %v", display.ErrConnection, err)
	}

	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(request); err != nil {
		return nil, fmt.Errorf("niri: write request: %w: %v", display.ErrConnection, err)
	}
	if err := writer.WriteByte('\n'); err != nil {
		return nil, fmt.Errorf("niri: write request: %w: %v", display.ErrConnection, err)
	}
	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("niri: write request: %w: %v", display.ErrConnection, err)
	}

	maxReply := c.cfg.MaxReplyBytes
	if maxReply <= 0 {
		maxReply = defaultMaxReplyBytes
	}
	initialBuffer := 64 * 1024
	if maxReply < initialBuffer {
		initialBuffer = maxReply
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, initialBuffer), maxReply)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("niri: read reply: %w: %v", display.ErrConnection, err)
		}
		return nil, fmt.Errorf("niri: read reply: %w: %v", display.ErrConnection, io.ErrUnexpectedEOF)
	}
	line := append([]byte(nil), scanner.Bytes()...)

	var envelope struct {
		Ok  json.RawMessage `json:"Ok"`
		Err *string         `json:"Err"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil, fmt.Errorf("niri: malformed reply: %w: %v", display.ErrConnection, err)
	}
	if envelope.Err != nil {
		return nil, &requestError{message: *envelope.Err}
	}
	if len(envelope.Ok) == 0 || string(envelope.Ok) == "null" {
		return nil, fmt.Errorf("niri: reply has no payload: %w", display.ErrUnsupported)
	}
	return envelope.Ok, nil
}

func decodePayload(raw json.RawMessage, target any) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("niri: decode reply: %w: %v", display.ErrUnsupported, err)
	}
	return nil
}
