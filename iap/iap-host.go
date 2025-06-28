package iap

import (
	"net/url"
	"strconv"

	mapstructure "github.com/go-viper/mapstructure/v2"
)

type IAPHost struct {
	ProjectID string `mapstructure:"project"`
	Zone      string `mapstructure:"zone"`
	Instance  string `mapstructure:"instance"`
	Interface string `mapstructure:"interface"`
	Port      string `mapstructure:"port"`
}

type reconnectParams struct {
	Ack  string `mapstructure:"ack"`
	Sid  string `mapstructure:"sid"`
	Zone string `mapstructure:"zone"`
}

func (h *IAPHost) ConnectURI() string {
	return tunnelURI(ConnectPath, h)
}

func (h *IAPHost) ReconnectURI(sid, ack uint64) string {
	return tunnelURI(ReconnectPath, &reconnectParams{
		Ack:  strconv.FormatUint(ack, 10),
		Sid:  strconv.FormatUint(sid, 10),
		Zone: h.Zone,
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
