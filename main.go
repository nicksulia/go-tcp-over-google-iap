package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nicksulia/go-tcp-over-google-iap/credentials"
	"github.com/nicksulia/go-tcp-over-google-iap/iap"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2/google"
)

type IAPHost struct {
	ProjectID string `mapstructure:"project"`
	Zone      string `mapstructure:"zone"`
	Instance  string `mapstructure:"instance"`
	Interface string `mapstructure:"interface"`
	Port      string `mapstructure:"port"`
}

var (
	projectID       string
	zone            string
	instance        string
	iface           string
	port            string
	localPort       string
	credentialsFile string
)

var rootCmd = &cobra.Command{
	Use:   "go-tcp-over-google-iap",
	Short: "TCP tunneling over Google IAP",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var creds *google.Credentials
		var err error

		if credentialsFile != "" {
			creds, err = credentials.ReadCredentialsFile(ctx, credentialsFile)
		} else {
			creds, err = credentials.DefaultCredentials(ctx)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading credentials file: %v\n", err)
		}

		host := iap.IAPHost{
			ProjectID: projectID,
			Zone:      zone,
			Instance:  instance,
			Interface: iface,
			Port:      port,
		}

		client, err := iap.NewIAPTunnelClient(host, creds, localPort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating IAP client: %v\n", err)
			os.Exit(1)
		}

		err = client.DryRun()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error during dry run: %v\n", err)
			os.Exit(1)
		}

		err = client.Serve(ctx)
		if err != nil {
			os.Exit(1)
		}

		// Handle SIGINT/SIGTERM for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			<-sigCh
			fmt.Println("Shutting down...")
			cancel()
			client.Close()
		}()
	},
}

func main() {
	rootCmd.Flags().StringVar(&projectID, "project", "", "GCP project ID")
	rootCmd.Flags().StringVar(&zone, "zone", "", "GCP zone")
	rootCmd.Flags().StringVar(&instance, "instance", "", "GCE instance name")
	rootCmd.Flags().StringVar(&iface, "interface", "nic0", "Network interface")
	rootCmd.Flags().StringVar(&port, "port", "22", "Port to connect to")
	rootCmd.Flags().StringVar(&localPort, "local-port", "2223", "Local port to bind for tunneling")
	rootCmd.Flags().StringVar(&credentialsFile, "credentials-file", "", "Absolute path to GCP service account credentials file (optional)")
	rootCmd.MarkFlagRequired("project")
	rootCmd.MarkFlagRequired("zone")
	rootCmd.MarkFlagRequired("instance")

	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}
