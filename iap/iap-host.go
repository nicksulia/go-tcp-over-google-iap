package iap

import (
	"fmt"
	"net/url"

	mapstructure "github.com/go-viper/mapstructure/v2"
)

// IAPHost represents the configuration for connecting to a Google Cloud IAP tunnel.
type IAPHost struct {
	ProjectID    string `mapstructure:"project"`
	Zone         string `mapstructure:"zone"`
	Instance     string `mapstructure:"instance"`
	Interface    string `mapstructure:"interface"`
	Port         string `mapstructure:"port"`
	NewWebsocket string `mapstructure:"newWebsocket,omitempty"`
}

type reconnectParams struct {
	Ack          string `mapstructure:"ack"`
	Sid          string `mapstructure:"sid"`
	Zone         string `mapstructure:"zone"`
	NewWebsocket string `mapstructure:"newWebsocket"`
}

// ConnectURI generates the URI for establishing a new connection to the IAP tunnel.
func (h *IAPHost) ConnectURI() string {
	h.NewWebsocket = "True"
	return tunnelURI(ConnectPath, &h)
}

// ReconnectURI is intended to restore existing session
func (h *IAPHost) ReconnectURI(sid string, ack uint64) string {
	return tunnelURI(ReconnectPath, &reconnectParams{
		Ack:          fmt.Sprintf("%d", ack),
		Sid:          sid,
		Zone:         h.Zone,
		NewWebsocket: "True",
	})
}

func tunnelURI(path string, src any) string {
	u := url.URL{
		Scheme:   WebSocketProtocol,
		Host:     IAPHostURL,
		Path:     path,
		RawQuery: queryParams(src).Encode(),
	}

	return u.String()
}

func queryParams(src any) url.Values {
	var params = make(map[string]string)
	mapstructure.Decode(src, &params)
	res := url.Values{}
	for k, v := range params {
		res.Add(k, v)
	}

	return res
}
