package iap

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateConnectURI(t *testing.T) {
	host := IAPHost{
		ProjectID: "test-project",
		Zone:      "us-central1-a",
		Instance:  "test-instance",
		Port:      "8080",
		Interface: "nic0",
	}

	expectedURI := "wss://tunnel.cloudproxy.app/v4/connect?instance=test-instance&interface=nic0&port=8080&project=test-project&zone=us-central1-a"
	assert.Equal(t, expectedURI, host.ConnectURI())
}

func TestCreateReconnectURI(t *testing.T) {
	host := IAPHost{
		Zone: "us-central1-a",
	}

	sid := uint64(12345)
	ack := uint64(67890)

	expectedURI := "wss://tunnel.cloudproxy.app/v4/reconnect?ack=67890&sid=12345&zone=us-central1-a"
	assert.Equal(t, expectedURI, host.ReconnectURI(sid, ack))
}

func TestQueryParams(t *testing.T) {
	host := IAPHost{
		ProjectID: "test-project",
		Zone:      "us-central1-a",
		Instance:  "test-instance",
		Port:      "8080",
		Interface: "nic0",
	}

	expectedParams := url.Values{
		"project":   {"test-project"},
		"zone":      {"us-central1-a"},
		"instance":  {"test-instance"},
		"port":      {"8080"},
		"interface": {"nic0"},
	}

	assert.Equal(t, expectedParams, queryParams(host))
}
