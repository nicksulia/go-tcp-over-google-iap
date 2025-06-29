// Package iap provides functionality for establishing TCP tunnels over Google Cloud IAP (Identity-Aware Proxy).
//
// This package includes an IAPTunnelClient that listens for local TCP connections and proxies them over
// secure IAP tunnels to remote Google Cloud VM instances. It manages authentication using Google credentials,
// handles connection retries, and synchronizes data between local and remote endpoints.
//
// Key types and functions:
//   - IAPTunnelClient: Manages the lifecycle of the TCP-over-IAP tunnel client, including listener setup,
//     connection handling, and tunnel management.
//   - NewIAPTunnelClient: Constructs a new IAPTunnelClient with the specified host, credentials, and local port.
//   - DryRun: Tests the connection to the IAP tunnel without establishing a full proxy.
//   - Serve: Starts the listener and handles incoming connections, spawning a new IAP tunnel for each.
//   - Close: Closes the listener and cleans up resources.
//
// Usage:
//  1. Create an IAPHost describing the target VM instance.
//  2. Obtain Google credentials (e.g., via ADC).
//  3. Instantiate IAPTunnelClient using NewIAPTunnelClient.
//  4. Call Serve to start accepting and proxying connections.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	creds, _ := credentials.DefaultCredentials(ctx)
//	host := IAPHost{Project: "my-project", Zone: "us-central1-a", Instance: "my-vm"}
//	client, _ := NewIAPTunnelClient(host, creds, "2201", nil)
//	err := client.DryRun(ctx) // Optional: Test the connection
//	if err == nil {
//		client.Serve(ctx)
//	}
package iap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nicksulia/go-tcp-over-google-iap/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
)

func withKeepAlive(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Minute)
	}
}

// tcpListener is a wrapper around net.Listener that adds retry logic for accepting connections.
type tcpListener struct {
	lis        net.Listener
	retryCount int
	counter    int
}

// Close closes the TCP listener and releases resources.
func (l *tcpListener) Close() error {
	return l.lis.Close()
}

func (l *tcpListener) Addr() net.Addr {
	return l.lis.Addr()
}

// Accept waits for and returns the next incoming connection to the listener.
func (l *tcpListener) Accept() (net.Conn, error, bool) {
	conn, err := l.lis.Accept()
	if err != nil {
		isClosed := isConnectionClosed(err)
		if isClosed {
			return nil, nil, isClosed
		}

		if l.counter < l.retryCount {
			l.counter++
			time.Sleep(time.Second * time.Duration(l.counter))
			return l.Accept() // Retry accepting connection
		}
		return nil, fmt.Errorf("failed to accept connection after retries: %w", err), false
	}
	withKeepAlive(conn)
	l.counter = 0 // Reset counter on successful accept
	return conn, nil, false
}

// newListener creates a new TCP listener wrapper on the specified port with retry logic.
func newListener(ctx context.Context, port string) (*tcpListener, error) {
	addr := fmt.Sprintf(":%s", port)
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener on port %s: %w", port, err)
	}

	return &tcpListener{
		lis:        lis,
		retryCount: 3,
	}, nil
}

// IAPTunnelClient manages a TCP-over-IAP tunnel client that listens for local connections
type IAPTunnelClient struct {
	logger      logger.Logger
	mu          sync.Mutex
	active      bool
	tokenSource oauth2.TokenSource
	host        IAPHost
	localPort   string
	lis         *tcpListener
}

// getHost is thread-safe method to retrieve the IAP host configuration.
func (c *IAPTunnelClient) getHost() IAPHost {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

// getTokenSource is a thread-safe method to retrieve the token source for authentication.
func (c *IAPTunnelClient) getTokenSource() oauth2.TokenSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokenSource
}

func (c *IAPTunnelClient) getLogger() logger.Logger {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logger
}

// setActive is a thread-safe method to set the active state of the IAPTunnelClient.
func (c *IAPTunnelClient) setActive(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = active
}

// isActive is a thread-safe method which checks if the IAPTunnelClient is currently active.
func (c *IAPTunnelClient) isActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Close is a thread-safe method to close the TCP listener and clean up resources.
func (c *IAPTunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lis != nil {
		return c.lis.Close()
	}
	return nil
}

// DryRun tests the connection to the IAP tunnel without establishing a full proxy.
// It attempts to connect to the IAP tunnel and returns any errors encountered.
func (c *IAPTunnelClient) DryRun() error {
	tunnel := NewIAPTunnel(c.getHost(), c.getTokenSource(), c.getLogger())
	return tunnel.DryRun(context.Background())
}

// Serve starts the TCP-over-IAP listener and handles incoming connections.
func (c *IAPTunnelClient) Serve(ctx context.Context) error {
	var err error
	if c.isActive() {
		return errors.New("tunnel client is already active")
	}
	c.setActive(true)
	defer c.setActive(false)

	if c.lis == nil {
		c.lis, err = newListener(ctx, c.localPort)
		if err != nil {
			return err
		}
	}

	defer c.lis.Close()

	defer func() {
		c.logger.Info("TCP-over-IAP listener closed, shutting down")
	}()

	c.logger.Info("TCP-over-IAP listener is ready", "addr", c.lis.Addr().String())
	for {
		conn, err, connClosed := c.lis.Accept()
		if connClosed {
			return nil // Listener closed, exit gracefully
		}

		if err != nil {
			c.logger.Error("Accept error", "err", err)
			return err
		}

		go c.processConn(ctx, conn)
	}
}

// processConn handles a new connection by establishing an IAP tunnel and synchronizing data between the connection and the tunnel.
// each TCP connection receives a new IAP tunnel instance.
func (c *IAPTunnelClient) processConn(ctx context.Context, conn net.Conn) {
	c.logger.Info("New connection accepted", "remote_addr", conn.RemoteAddr().String())
	tunnel := NewIAPTunnel(c.getHost(), c.getTokenSource(), c.getLogger())
	tunnel.Start(ctx)
	defer tunnel.Close()
	defer conn.Close()

	select {
	case <-tunnel.Ready():
		// Tunnel is ready
	case <-ctx.Done():
		return
	}

	err := syncConnections(ctx, conn, tunnel)
	if err != nil && !isConnectionClosed(err) {
		c.logger.Error("Proxy error", "err", err)
	}
}

// isConnectionClosed checks if the error indicates that the listener's connection has been closed.
func isConnectionClosed(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF)
}

func copyConn(target, source io.ReadWriteCloser) func() error {
	return func() error {
		_, err := io.Copy(target, source)
		return err
	}
}

// syncConnections synchronizes data between two connections.
func syncConnections(ctx context.Context, source, target io.ReadWriteCloser) error {
	g, _ := errgroup.WithContext(ctx)
	g.Go(copyConn(target, source))
	g.Go(copyConn(source, target))
	return g.Wait()
}

// NewIAPTunnelClient creates a new IAPTunnelClient with the specified host, credentials, and local port.
// It initializes the client with default values if not provided, and validates the credentials.
// Example usage:
//
//	host := IAPHost{ProjectID: "my-project", Zone: "us-central1-a", Instance: "my-instance"}
//	creds, _ := google.FindDefaultCredentials(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
//	client, _ := NewIAPTunnelClient(host, creds, "2201", nil)
//	client.Serve(context.Background())
func NewIAPTunnelClient(host IAPHost, creds *google.Credentials, localPort string, l logger.Logger) (*IAPTunnelClient, error) {
	client := &IAPTunnelClient{
		host:      host,
		localPort: localPort,
		logger:    l,
	}

	if client.logger == nil {
		client.logger, _ = logger.NewZapLogger("info") // Default logger if none provided
	}

	if client.host.Instance == "" {
		client.host.Interface = "nic0"
	}

	if client.localPort == "" {
		client.localPort = "2201" // Default local port if not specified
	}

	if creds == nil {
		return nil, errors.New("google credentials cannot be nil")
	}

	client.tokenSource = creds.TokenSource
	if client.tokenSource == nil {
		return nil, errors.New("google credentials token source cannot be nil")
	}

	return client, nil
}
