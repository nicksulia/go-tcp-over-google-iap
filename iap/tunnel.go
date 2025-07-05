package iap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/nicksulia/go-tcp-over-google-iap/logger"
	"golang.org/x/oauth2"
)

type IAPTunnel struct {
	ws                      *websocket.Conn
	host                    IAPHost
	tokenSource             oauth2.TokenSource
	totalBytesConfirmed     uint64
	sid                     string
	logger                  logger.Logger
	incoming                chan []byte
	totalBytesReceived      uint64
	totalBytesReceivedAcked uint64
	msgBuffer               []byte
	closed                  chan struct{}
	ready                   chan struct{}
	readyOnce               sync.Once
}

// NewIAPTunnel creates a new IAPTunnel instance with the specified host and token source.
// It initializes the incoming channel for receiving data and sets up channels for closed and ready states.
func NewIAPTunnel(host IAPHost, source oauth2.TokenSource, logger logger.Logger) *IAPTunnel {
	return &IAPTunnel{
		host:        host,
		tokenSource: source,
		incoming:    make(chan []byte, 1024),
		closed:      make(chan struct{}),
		ready:       make(chan struct{}),
		logger:      logger,
	}
}

func (t *IAPTunnel) headers() (http.Header, error) {
	token, err := t.tokenSource.Token()
	if err != nil {
		return nil, err
	}

	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	h.Add("Origin", Origin)
	h.Add("User-Agent", UserAgent)
	return h, nil
}

func (t *IAPTunnel) connectOrReconnect(ctx context.Context) (*websocket.Conn, *http.Response, error) {
	var err error
	var res *http.Response
	// Reset state for a new connection
	t.totalBytesReceived = 0
	t.totalBytesReceivedAcked = 0
	t.totalBytesConfirmed = 0
	u := t.host.ConnectURI()
	t.logger.Info("Connecting to IAP Tunnel", "URI", u)

	headers, err := t.headers()
	if err != nil {
		return nil, nil, err
	}
	opts := &websocket.DialOptions{
		HTTPHeader:   headers,
		Subprotocols: []string{RelayProtocolName},
	}

	t.ws, res, err = websocket.Dial(ctx, u, opts)

	return t.ws, res, err
}

// DryRun tests the connection to the IAP tunnel without establishing a full proxy.
// It attempts to connect to the IAP tunnel and returns any errors encountered.
func (t *IAPTunnel) DryRun(ctx context.Context) error {
	_, _, err := t.connectOrReconnect(ctx)
	if err != nil {
		return err
	}

	_, _, err = t.ws.Read(ctx) // Read to ensure connection is established
	if err != nil {
		return err
	}

	t.logger.Info("Dry run successful, connection established.")
	t.Close()
	return nil
}

// Start initiates goroutine to start the IAP tunnel connection and read messages.
func (t *IAPTunnel) Start(ctx context.Context) {
	go t.start(ctx)
}

// start initiates the IAP tunnel connection and begins reading messages.
// It handles reconnections if the connection is lost.
func (t *IAPTunnel) start(ctx context.Context) {
	_, _, err := t.connectOrReconnect(ctx)
	if err != nil {
		t.logger.Error("Connect failed", "err", err)
		return
	}

	for {
		_, msg, err := t.ws.Read(ctx)

		select {
		case <-ctx.Done():
			t.logger.Info("Context cancelled, stopping read loop")
			return
		case <-t.closed:
			t.logger.Info("Tunnel closed, stopping read loop")
			return
		default:
		}

		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				t.logger.Info("Websocket closed normally")
				return
			}

			t.logger.Error("Websocket read error", "err", err)
			// Attempt reconnect if not context cancellation
			if ctx.Err() == nil && t.sid != "" {
				_, _, err = t.connectOrReconnect(ctx)
				if err != nil {
					t.logger.Error("Reconnect failed", "err", err)
					return
				}

				continue
			}

			return
		}

		t.handleFrame(NewIncomingFrame(msg))
	}
}

// handleFrame processes incoming frames based on their type.
func (t *IAPTunnel) handleFrame(frame *IncomingFrame) {
	switch frame.Type() {
	case RelayConnectSuccessSID:
		t.handleConnectSuccessSID(frame)
	case RelayReconnectSuccessACK:
		t.handleReconnectSuccessACK(frame)
	case RelayACK:
		t.handleACK(frame)
	case RelayData:
		t.handleData(frame)
	default:
		t.logger.Warn("Unknown frame type: ", frame.Type())
	}
}

// handleConnectSuccessSID processes incoming connect success SID frames.
func (t *IAPTunnel) handleConnectSuccessSID(frame *IncomingFrame) {
	t.sid = frame.SID()
	t.logger.Info("Connect success")
	t.logger.Debug("Session Details", "SID", t.sid)
	t.readyOnce.Do(func() {
		close(t.ready)
	})
}

// handleReconnectSuccessACK processes incoming reconnect success ACK frames.
func (t *IAPTunnel) handleReconnectSuccessACK(frame *IncomingFrame) {
	t.totalBytesConfirmed = frame.ACK()
	t.logger.Debug("Reconnect success", "ACK Bytes", t.totalBytesConfirmed)
}

// handleACK processes incoming ACK frames.
func (t *IAPTunnel) handleACK(frame *IncomingFrame) {
	t.totalBytesConfirmed = frame.ACK()
	t.logger.Debug("ACK received", "ACK Bytes", t.totalBytesConfirmed)
}

// handleData processes incoming data frames.
func (t *IAPTunnel) handleData(frame *IncomingFrame) {
	data, rest := frame.Data()
	// Process the data as needed
	t.logger.Debug("Data received", "Data Length", len(data), "binary_data[:20]", frame.data[:20])
	if data != nil {
		t.incoming <- data
		t.totalBytesReceived += uint64(len(data))
		// gcloud iap-tunnel client sends ACKs for every MaxMessageSize * 2  bytes received
		if t.totalBytesReceived-t.totalBytesReceivedAcked > MaxMessageSize*2 {
			_, err := NewACKFrame(t.totalBytesReceived, t.logger).Send(t.ws)
			if err != nil {
				t.logger.Debug("Failed to send ACK frame", "err", err)
				return
			}

			t.totalBytesReceivedAcked = t.totalBytesReceived
		}
	}

	// If there is additional data, handle it accordingly
	if len(rest) > 0 {
		t.logger.Debug("Discard additional data received after main payload", "Length", len(rest))
	}
}

// Ready returns a channel that is closed when the tunnel is ready to accept data.
func (t *IAPTunnel) Ready() <-chan struct{} {
	return t.ready
}

// Read implements the io.Reader interface for IAPTunnel.
func (t *IAPTunnel) Read(p []byte) (int, error) {
	select {
	case <-t.closed:
		return 0, io.EOF
	case <-t.ready:
	}

	// Serve any pending data first
	if len(t.msgBuffer) > 0 {
		n := copy(p, t.msgBuffer)
		t.msgBuffer = t.msgBuffer[n:]
		return n, nil
	}

	data, ok := <-t.incoming
	if !ok {
		return 0, io.EOF
	}

	n := copy(p, data)
	// buffer is empty, so we can copy the data directly
	t.msgBuffer = data[n:]
	return n, nil
}

// Write implements the io.Writer interface for IAPTunnel.
func (t *IAPTunnel) Write(p []byte) (n int, err error) {
	payloadLen := len(p)
	totalSent := 0

	for totalSent < len(p) {
		chunkEnd := totalSent + MaxMessageSize
		if chunkEnd > payloadLen {
			chunkEnd = payloadLen
		}

		// Avoid slicing multiple times
		chunk := p[totalSent:chunkEnd]
		frame := NewDataFrame(chunk, t.logger)

		var sent int
		var sendErr error
		if sent, sendErr = frame.Send(t.ws); sendErr != nil {
			return totalSent, sendErr
		}

		totalSent += sent
	}

	return totalSent, nil
}

// Close implements the io.Closer interface for IAPTunnel.
func (t *IAPTunnel) Close() error {
	close(t.closed)
	if t.ws != nil {
		return t.ws.Close(websocket.StatusNormalClosure, "closing IAP tunnel")
	}
	return nil
}
