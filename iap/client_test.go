package iap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/nicksulia/go-tcp-over-google-iap/credentials"
	"github.com/stretchr/testify/assert"
)

// TestIAPClientE2E serves as a simple solution to debug and troubleshoot
func TestIAPClientE2E(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "true" {
		t.Skip("Skipping Integration tests. TEST_INTEGRATION is not 'true'")
	}

	// GOOGLE_APPLICATION_CREDENTIALS will work with
	pathToCreds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	creds, err := credentials.ReadCredentialsFile(context.Background(), pathToCreds)
	assert.NoError(t, err, "ReadCredentialsFile should not respond with error")
	host := IAPHost{
		ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT_ID"),
		Zone:      os.Getenv("GOOGLE_CLOUD_INSTANCE_ZONE"),
		Instance:  os.Getenv("GOOGLE_CLOUD_INSTANCE_NAME"),
		Port:      os.Getenv("GOOGLE_CLOUD_INSTANCE_PORT"),
		Interface: "nic0",
	}

	client, err := NewIAPTunnelClient(host, "3089")
	assert.Nil(t, err)
	err = client.SetCredentials(creds)
	assert.Nil(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("Shutting down...")
		cancel()
	}()
	err = client.Serve(ctx)
	assert.Nil(t, err)
}
