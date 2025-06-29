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
//	client, _ := NewIAPTunnelClient(host, creds, "2201")
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

type tcpListener struct {
	lis        net.Listener
	retryCount int
	counter    int
}

func (l *tcpListener) Close() error {
	return l.lis.Close()
}

func (l *tcpListener) Accept() (net.Conn, error, bool) {
	conn, err := l.lis.Accept()
	if err != nil {
		isClosed := isConnectionClosed(err)
		if isClosed {
			return nil, nil, isClosed
		}

		if l.counter < l.retryCount {
			l.counter++
			fmt.Println("Accept error:", err, "retrying: ", l.counter, "/", l.retryCount)
			time.Sleep(time.Second * time.Duration(l.counter))
			return l.Accept() // Retry accepting connection
		}
		return nil, fmt.Errorf("failed to accept connection after retries: %w", err), false
	}
	withKeepAlive(conn)
	l.counter = 0 // Reset counter on successful accept
	return conn, nil, false
}

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

type IAPTunnelClient struct {
	mu          sync.Mutex
	active      bool
	tokenSource oauth2.TokenSource
	host        IAPHost
	localPort   string
	lis         *tcpListener
}

func (c *IAPTunnelClient) getHost() IAPHost {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

func (c *IAPTunnelClient) getTokenSource() oauth2.TokenSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokenSource
}

func (c *IAPTunnelClient) setActive(active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = active
}

func (c *IAPTunnelClient) isActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

func (c *IAPTunnelClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lis != nil {
		return c.lis.Close()
	}
	return nil
}

func (c *IAPTunnelClient) DryRun() error {
	tunnel := NewIAPTunnel(c.host, c.tokenSource)
	return tunnel.DryRun(context.Background())
}

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
		fmt.Println("TCP-over-IAP listener closed, shutting down")
	}()

	fmt.Println("TCP-over-IAP listener is ready", c.lis.lis.Addr().String())
	for {
		conn, err, connClosed := c.lis.Accept()
		if connClosed {
			return nil // Listener closed, exit gracefully
		}

		if err != nil {

			fmt.Println("Accept error", "err", err)
			return err
		}

		go c.processConn(ctx, conn)
	}
}

// processConn handles a new connection by establishing an IAP tunnel and synchronizing data between the connection and the tunnel.
// each TCP connection receives a new IAP tunnel instance.
func (c *IAPTunnelClient) processConn(ctx context.Context, conn net.Conn) {
	fmt.Println("New connection accepted", "remote_addr", conn.RemoteAddr().String())
	tunnel := NewIAPTunnel(c.getHost(), c.getTokenSource())
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
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		fmt.Println("Proxy error", "err", err)
	}
}

func isConnectionClosed(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed)
}

func copyConn(source, target io.ReadWriteCloser) func() error {
	return func() error {
		_, err := io.Copy(source, target)
		return err
	}
}

// syncConnections synchronizes data between two connections.
func syncConnections(ctx context.Context, source, target io.ReadWriteCloser) error {
	g, _ := errgroup.WithContext(ctx)
	g.Go(copyConn(source, target))
	g.Go(copyConn(target, source))
	return g.Wait()
}

func NewIAPTunnelClient(host IAPHost, creds *google.Credentials, localPort string) (*IAPTunnelClient, error) {
	if host.Interface == "" {
		host.Interface = "nic0"
	}

	if localPort == "" {
		localPort = "2201" // Default local port if not specified
	}

	if creds == nil {
		return nil, errors.New("google credentials cannot be nil")
	}

	tokenSource := creds.TokenSource
	if tokenSource == nil {
		return nil, errors.New("google credentials token source cannot be nil")
	}

	return &IAPTunnelClient{
		tokenSource: tokenSource,
		host:        host,
		localPort:   localPort,
		active:      false,
	}, nil
}
