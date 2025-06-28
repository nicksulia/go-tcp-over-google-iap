package iap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2/google"
)

func credentials() (*google.Credentials, error) {
	// by default provide sa key as path
	return google.FindDefaultCredentials(
		context.Background(),
		"https://www.googleapis.com/auth/compute",
		"https://www.googleapis.com/auth/cloud-platform",
	)
}

// TestIAPClientE2E serves as a simple solution to debug and troubleshoot
func TestIAPClientE2E(t *testing.T) {
	if os.Getenv("TEST_INTEGRATION") != "true" {
		t.Skip("Skipping Integration tests. TEST_INTEGRATION is not 'true'")
	}

	// GOOGLE_APPLICATION_CREDENTIALS will work with
	creds, _ := credentials()
	host := IAPHost{
		ProjectID: os.Getenv("GOOGLE_CLOUD_PROJECT_ID"),
		Zone:      os.Getenv("GOOGLE_CLOUD_INSTANCE_ZONE"),
		Instance:  os.Getenv("GOOGLE_CLOUD_INSTANCE_NAME"),
		Port:      os.Getenv("GOOGLE_CLOUD_INSTANCE_PORT"),
		Interface: "nic0",
	}

	client, err := NewIAPTunnelClient(host, creds, "2223")
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
