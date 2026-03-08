package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	writeMsg1   = `{"bid":42}`
	readMsg1    = "msg-1"
	readMsg2    = "msg-2"
	pumpTimeout = 2 * time.Second
)

type clientSuite struct {
	suite.Suite
}

func TestClientSuite(t *testing.T) {
	suite.Run(t, new(clientSuite))
}

// dialPair creates a real WebSocket connection pair: a test HTTP server that
// accepts a WS upgrade and returns the server-side conn, and a dialed
// client-side conn.
func dialPair(t *testing.T) (serverConn *websocket.Conn, clientConn *websocket.Conn) {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		serverConnCh <- conn
		// Keep the handler alive until the test ends.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):]
	cConn, resp, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	sConn := <-serverConnCh
	return sConn, cConn
}

// ---------------------------------------------------------------------------
// WritePump
// ---------------------------------------------------------------------------

func (s *clientSuite) TestWritePumpDeliversMessageToClientConn() {
	serverConn, clientConn := dialPair(s.T())
	defer func() { _ = serverConn.CloseNow() }()
	defer func() { _ = clientConn.CloseNow() }()

	c := NewClient(serverConn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.WritePump(ctx)

	recv := make(chan []byte, 1)
	go func() {
		_, msg, err := clientConn.Read(context.Background())
		if err == nil {
			recv <- msg
		}
	}()

	c.send <- []byte(writeMsg1)

	select {
	case msg := <-recv:
		s.Equal(writeMsg1, string(msg))
	case <-time.After(pumpTimeout):
		s.Fail("timed out waiting for message")
	}
}

func (s *clientSuite) TestWritePumpExitsWhenSendChannelClosed() {
	serverConn, clientConn := dialPair(s.T())
	defer func() { _ = clientConn.CloseNow() }()

	c := NewClient(serverConn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.WritePump(ctx)
		close(done)
	}()

	close(c.send)

	select {
	case <-done:
	case <-time.After(pumpTimeout):
		s.Fail("WritePump did not exit after send channel close")
	}
}

func (s *clientSuite) TestWritePumpExitsOnContextCancellation() {
	serverConn, clientConn := dialPair(s.T())
	defer func() { _ = clientConn.CloseNow() }()

	c := NewClient(serverConn)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.WritePump(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(pumpTimeout):
		s.Fail("WritePump did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// ReadPump
// ---------------------------------------------------------------------------

func (s *clientSuite) TestReadPumpInvokesHandlerForEachMessage() {
	serverConn, clientConn := dialPair(s.T())
	defer func() { _ = serverConn.CloseNow() }()
	defer func() { _ = clientConn.CloseNow() }()

	c := NewClient(serverConn)

	recv := make(chan []byte, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.ReadPump(ctx, func(msg []byte) { recv <- msg })

	err := clientConn.Write(context.Background(), websocket.MessageText, []byte(readMsg1))
	s.NoError(err)

	select {
	case msg := <-recv:
		s.Equal(readMsg1, string(msg))
	case <-time.After(pumpTimeout):
		s.Fail("timed out waiting for msg-1")
	}

	err = clientConn.Write(context.Background(), websocket.MessageText, []byte(readMsg2))
	s.NoError(err)

	select {
	case msg := <-recv:
		s.Equal(readMsg2, string(msg))
	case <-time.After(pumpTimeout):
		s.Fail("timed out waiting for msg-2")
	}
}

func (s *clientSuite) TestReadPumpExitsWhenConnectionClosed() {
	serverConn, clientConn := dialPair(s.T())

	c := NewClient(serverConn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.ReadPump(ctx, func(_ []byte) {})
		close(done)
	}()

	clientConn.Close(websocket.StatusNormalClosure, "bye")

	select {
	case <-done:
	case <-time.After(pumpTimeout):
		s.Fail("ReadPump did not exit after connection close")
	}
}
