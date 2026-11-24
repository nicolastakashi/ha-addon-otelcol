package homeassistantreceiver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// haClient is a minimal, single-goroutine Home Assistant WebSocket API client.
type haClient struct {
	conn *websocket.Conn
	id   int
}

func dial(ctx context.Context, endpoint, token string) (*haClient, error) {
	conn, _, err := websocket.Dial(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", endpoint, err)
	}
	conn.SetReadLimit(64 << 20) // traces with many steps can be large

	var msg struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil || msg.Type != "auth_required" {
		conn.CloseNow()
		return nil, fmt.Errorf("expected auth_required, got %q: %v", msg.Type, err)
	}
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "auth", "access_token": token}); err != nil {
		conn.CloseNow()
		return nil, err
	}
	if err := wsjson.Read(ctx, conn, &msg); err != nil {
		conn.CloseNow()
		return nil, err
	}
	if msg.Type != "auth_ok" {
		conn.CloseNow()
		return nil, fmt.Errorf("authentication failed: %s %s", msg.Type, msg.Message)
	}
	return &haClient{conn: conn}, nil
}

// call sends a command and decodes its result into out.
func (c *haClient) call(ctx context.Context, cmd map[string]any, out any) error {
	c.id++
	cmd["id"] = c.id
	if err := wsjson.Write(ctx, c.conn, cmd); err != nil {
		return err
	}
	for {
		var msg struct {
			ID      int             `json:"id"`
			Type    string          `json:"type"`
			Success bool            `json:"success"`
			Result  json.RawMessage `json:"result"`
			Error   *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := wsjson.Read(ctx, c.conn, &msg); err != nil {
			return err
		}
		if msg.ID != c.id || msg.Type != "result" {
			continue // not ours (we never subscribe, but stay tolerant)
		}
		if !msg.Success {
			if msg.Error != nil {
				return fmt.Errorf("%v: %s: %s", cmd["type"], msg.Error.Code, msg.Error.Message)
			}
			return fmt.Errorf("%v failed", cmd["type"])
		}
		return json.Unmarshal(msg.Result, out)
	}
}

func (c *haClient) close() { c.conn.CloseNow() }
