package iap

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/coder/websocket"
	"golang.org/x/oauth2"
)

type IAPTunnel struct {
	ws                      *websocket.Conn
	host                    IAPHost
	tokenSource             oauth2.TokenSource
	totalBytesConfirmed     uint64
	sid                     string
	incoming                chan []byte
	totalBytesReceived      uint64
	totalBytesReceivedAcked uint64
	msgBuffer               []byte
	closed                  chan struct{}
	ready                   chan struct{}
}

func NewIAPTunnel(host IAPHost, source oauth2.TokenSource) *IAPTunnel {
	return &IAPTunnel{
		host:        host,
		tokenSource: source,
		incoming:    make(chan []byte, 1024),
		closed:      make(chan struct{}),
		ready:       make(chan struct{}),
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
	fmt.Println("Connecting to IAP Tunnel", "URI", u)

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

func (t *IAPTunnel) DryRun(ctx context.Context) error {
	_, _, err := t.connectOrReconnect(ctx)
	if err != nil {
		fmt.Println("Dry run connect failed", "err", err)
		return err
	}

	_, _, err = t.ws.Read(ctx) // Read to ensure connection is established
	if err != nil {
		fmt.Println("Dry run read failed", "err", err)
		return err
	}

	fmt.Println("Dry run successful, connection established.")
	t.Close()
	return nil
}

func (t *IAPTunnel) Start(ctx context.Context) {
	go t.start(ctx)
}

func (t *IAPTunnel) start(ctx context.Context) {
	_, _, err := t.connectOrReconnect(ctx)
	if err != nil {
		fmt.Println("Connect failed", "err", err)
		return
	}

	for {
		_, msg, err := t.ws.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				fmt.Println("Websocket closed normally")
				return
			}

			fmt.Println("Websocket read error", "err", err)
			// Attempt reconnect if not context cancellation
			if ctx.Err() == nil && t.sid != "" {
				_, _, err = t.connectOrReconnect(ctx)
				if err != nil {
					fmt.Println("Reconnect failed", "err", err)
					return
				}

				continue
			}

			fmt.Println("Read error", "err", err)
			return
		}

		t.handleFrame(NewIncomingFrame(msg))
	}
}

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
		fmt.Println("Unknown frame type: ", frame.Type())
	}
}

func (t *IAPTunnel) handleConnectSuccessSID(frame *IncomingFrame) {
	t.sid = frame.SID()
	fmt.Println("Connect success", "sid", t.sid)
	select {
	case <-t.ready:
		// already closed
	default:
		close(t.ready)
	}
}

func (t *IAPTunnel) handleReconnectSuccessACK(frame *IncomingFrame) {
	t.totalBytesConfirmed = frame.ACK()
	fmt.Println("Reconnect success", "ACK Bytes", t.totalBytesConfirmed)
}

func (t *IAPTunnel) handleACK(frame *IncomingFrame) {
	t.totalBytesConfirmed = frame.ACK()
	fmt.Println("ACK received", "ACK Bytes", t.totalBytesConfirmed)
}

func (t *IAPTunnel) handleData(frame *IncomingFrame) {
	data, rest := frame.Data()
	// Process the data as needed
	fmt.Println("Data received", "Data Length", len(data), "binary_data[:20]", frame.data[:20])
	if data != nil {
		t.incoming <- data
		t.totalBytesReceived += uint64(len(data))
		// gcloud iap-tunnel client sends ACKs for every MaxMessageSize * 2  bytes received
		if t.totalBytesReceived-t.totalBytesReceivedAcked > MaxMessageSize*2 {
			_, err := NewACKFrame(t.totalBytesReceived).Send(t.ws)
			if err != nil {
				fmt.Println("Failed to send ACK frame", "err", err)
				return
			}

			t.totalBytesReceivedAcked = t.totalBytesReceived
		}
	}

	// If there is additional data, handle it accordingly
	if len(rest) > 0 {
		fmt.Println("Discard additional data received after main payload", "Length", len(rest))
	}
}

func (t *IAPTunnel) Ready() <-chan struct{} {
	return t.ready
}

func (t *IAPTunnel) Read(p []byte) (n int, err error) {
	// Serve any pending data first
	if len(t.msgBuffer) > 0 {
		n := copy(p, t.msgBuffer)
		t.msgBuffer = t.msgBuffer[n:]
		return n, nil
	}

	select {
	case data, ok := <-t.incoming:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, data)
		// buffer is empty, so we can copy the data directly
		t.msgBuffer = data[n:]
		return n, nil // Indicate that no data read yet, but will be available in the next call
	case <-t.closed:
		return 0, io.EOF
	}
}

func (t *IAPTunnel) Write(p []byte) (n int, err error) {
	toSend := p
	payloadLen := len(p)

	if payloadLen > MaxMessageSize {
		toSend = p[:MaxMessageSize]
	}
	frame := NewDataFrame(toSend)

	return frame.Send(t.ws)
}

func (t *IAPTunnel) Close() error {
	close(t.closed)
	close(t.incoming)
	if t.ws != nil {
		return t.ws.Close(websocket.StatusNormalClosure, "closing IAP tunnel")
	}
	return nil
}
