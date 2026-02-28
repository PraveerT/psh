// Package client handles the TCP connection to the PhoneSSH daemon.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	DialTimeout    = 10 * time.Second
	CommandTimeout = 60 * time.Second
)

// Client is a connected session to a PhoneSSH daemon.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	device *Device
}

// ── Wire protocol types ──────────────────────────────────────────────────────

type HelloMsg struct {
	Type                 string `json:"type"`
	Version              string `json:"version"`
	DeviceName           string `json:"deviceName"`
	PhonePubkeyFingerprint string `json:"phonePubkeyFingerprint"`
}

type AuthMsg struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

type AuthOkMsg struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
}

type AuthFailMsg struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

type CmdMsg struct {
	Type    string            `json:"type"`
	ID      string            `json:"id"`
	Cmd     string            `json:"cmd"`
	Args    []string          `json:"args"`
	Flags   map[string]string `json:"flags"`
	Payload string            `json:"payload,omitempty"` // base64 for uploads
}

type ResultMsg struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Ok    bool                   `json:"ok"`
	Data  map[string]interface{} `json:"data"`
	Error string                 `json:"error"`
}

// ── Connect ─────────────────────────────────────────────────────────────────

// Connect dials the daemon, performs auth handshake, and returns a ready Client.
func Connect(device *Device) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", device.Host, device.Port)

	conn, err := net.DialTimeout("tcp", addr, DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w\n\nIs PhoneSSH running on your phone? Is the phone on the same network (or Tailscale)?", addr, err)
	}

	c := &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		device: device,
	}

	if err := c.handshake(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) handshake() error {
	c.conn.SetDeadline(time.Now().Add(DialTimeout))
	defer c.conn.SetDeadline(time.Time{})

	// Receive hello
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading hello: %w", err)
	}
	var hello HelloMsg
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &hello); err != nil {
		return fmt.Errorf("parsing hello: %w", err)
	}
	if hello.Type != "hello" {
		return fmt.Errorf("expected hello, got: %s", hello.Type)
	}

	// Send auth
	authMsg := AuthMsg{Type: "auth", Token: c.device.Token}
	if err := c.writeLine(authMsg); err != nil {
		return fmt.Errorf("sending auth: %w", err)
	}

	// Read response
	line, err = c.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading auth response: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &raw); err != nil {
		return fmt.Errorf("parsing auth response: %w", err)
	}

	switch raw["type"] {
	case "auth_ok":
		return nil
	case "auth_fail":
		return fmt.Errorf("authentication failed: %v\n\nYour token may be outdated — re-run: psh pair", raw["error"])
	default:
		return fmt.Errorf("unexpected auth response type: %v", raw["type"])
	}
}

// ── Commands ─────────────────────────────────────────────────────────────────

// Run sends a command and returns the parsed result.
func (c *Client) Run(cmd CmdMsg) (*ResultMsg, error) {
	c.conn.SetDeadline(time.Now().Add(CommandTimeout))
	defer c.conn.SetDeadline(time.Time{})

	if err := c.writeLine(cmd); err != nil {
		return nil, fmt.Errorf("sending command: %w", err)
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result ResultMsg
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &result, nil
}

// RunRaw sends a pre-built command message and returns the raw JSON response.
func (c *Client) RunRaw(cmd CmdMsg) (map[string]interface{}, error) {
	result, err := c.Run(cmd)
	if err != nil {
		return nil, err
	}
	if !result.Ok {
		return nil, fmt.Errorf("%s", result.Error)
	}
	return result.Data, nil
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) writeLine(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.conn, "%s\n", data)
	return err
}
