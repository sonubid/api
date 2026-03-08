package hub

import (
	"context"

	"github.com/coder/websocket"
)

const sendBufferSize = 256

// Client represents a single active WebSocket connection.
// Each client belongs to one auction room and owns a buffered send channel
// that the WritePump drains to write outbound messages to the connection.
// Client instances must be created via NewClient and their pumps started
// as goroutines immediately after registration in the Hub.
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// NewClient creates a Client wrapping the given WebSocket connection.
// The send channel is buffered to decouple the Hub's Broadcast call from
// the actual network write, preventing a slow client from blocking others.
func NewClient(conn *websocket.Conn) *Client {
	return &Client{
		conn: conn,
		send: make(chan []byte, sendBufferSize),
	}
}

// WritePump reads messages from the client's send channel and writes them
// to the WebSocket connection. It runs until the send channel is closed or
// the context is cancelled. WritePump must be run in its own goroutine.
func (c *Client) WritePump(ctx context.Context) {
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.CloseNow()
				return
			}

			if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}

		case <-ctx.Done():
			_ = c.conn.CloseNow()
			return
		}
	}
}

// ReadPump reads incoming messages from the WebSocket connection and passes
// each raw message to the provided handler function. It runs until the
// connection is closed or the context is cancelled. ReadPump must be run
// in its own goroutine. The handler is called synchronously within the
// read loop, so it must not block for extended periods.
func (c *Client) ReadPump(ctx context.Context, handler func([]byte)) {
	for {
		_, msg, err := c.conn.Read(ctx)
		if err != nil {
			return
		}

		handler(msg)
	}
}
