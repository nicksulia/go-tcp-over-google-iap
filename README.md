# TCP tunneling to GCE instances over Google Cloud IAP (Identity-Aware Proxy)

## Usage as CLI Tool

`go-tcp-over-google-iap` can be used as command-line tool for establishing a secure TCP tunnel over Google Cloud IAP to a Google Compute Engine instance. This enables SSH or any other TCP-based access without requiring external IP addresses.

### Installation

```bash
go install github.com/nicksulia/go-tcp-over-google-iap@latest
```

### Usage

```bash
go-tcp-over-google-iap \
  --project <GCP_PROJECT_ID> \
  --zone <GCP_ZONE> \
  --instance <GCE_INSTANCE_NAME> \
  [--interface <INTERFACE>] \
  [--port <REMOTE_PORT>] \
  [--local-port <LOCAL_PORT>] \
  [--credentials-file <PATH_TO_SERVICE_ACCOUNT_JSON>]
```

### Example

```bash
go-tcp-over-google-iap \
  --project my-gcp-project \
  --zone us-central1-a \
  --instance my-instance \
  --port 22 \
  --local-port 2223
```

Then you can SSH through the local port:

```bash
ssh -p 2223 username@localhost
```

| Flag                 | Description                                               | Default | Required |
| -------------------- | --------------------------------------------------------- | ------- | -------- |
| `--project`          | Google Cloud project ID                                   | —       | ✅       |
| `--zone`             | Zone of the GCE instance                                  | —       | ✅       |
| `--instance`         | Name of the GCE instance                                  | —       | ✅       |
| `--interface`        | Network interface (usually `nic0`)                        | `nic0`  | ❌       |
| `--port`             | Remote TCP port on the GCE instance                       | `22`    | ❌       |
| `--local-port`       | Local port to bind to                                     | `2223`  | ❌       |
| `--credentials-file` | Path to a service account JSON file (uses ADC if omitted) | —       | ❌       |
| `--loglevel`         | Logging level. Supports `debug`, `info`, `warn`, `error`  | `info`  | ❌       |

## Usage as a Library

Go library for creating TCP tunnels over Google Cloud Identity-Aware Proxy (IAP)

### Features

- Secure TCP tunneling to internal GCE instances via IAP
- Provides a `Logger` interface compatible with structured loggers (defaults to [zap](https://github.com/uber-go/zap))
- Supports custom ports, interfaces, and zones
- Graceful shutdown via context
- Dry-run support to validate setup

### Prerequisites

- **Go ≥ 1.21** (tested with Go 1.24+)

### Installation

```bash
go get github.com/nicksulia/go-tcp-over-google-iap
```

### Usage Example

```go
func main() {
	ctx := context.Background()

	// Define the GCE target
	host := iap.IAPHost{
		ProjectID: "my-project",
		Zone:      "us-central1-a",
		Instance:  "my-instance",
		Interface: "nic0",
		Port:      "22",
	}

	client, err := iap.NewIAPTunnelClient(host, "2223")
	if err != nil {
		log.Fatal("client error", "err", err)
	}

	if err := client.DryRun(); err != nil {
		log.Fatal("dry run failed", "err", err)
	}

	if err := client.Serve(ctx); err != nil {
		log.Fatal("serve failed", "err", err)
	}
}
```

### API Reference

```go
// logger/logger.go
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
	Fatal(msg string, keysAndValues ...any)
}

type IAPHost struct {
	ProjectID string
	Zone      string
	Instance  string
	Interface string
	Port      string
}

func NewIAPTunnelClient(host IAPHost, localPort string) (*IAPTunnelClient, error)
func (c *IAPTunnelClient) DryRun() error
func (c *IAPTunnelClient) Serve(ctx context.Context) error
func (c *IAPTunnelClient) SetCredentials(creds *google.Credentials) error
func (c *IAPTunnelClient) SetLogger(l logger.Logger) error
func (c *IAPTunnelClient) Close()
```
