package main

import (
	"github.com/nicksulia/go-tcp-over-google-iap/iap"
)

func main() {
	iap.NewIAPTunnelClient(iap.IAPHost{}, nil, "") // default init to pull required modules
}
