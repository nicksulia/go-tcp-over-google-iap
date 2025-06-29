package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nicksulia/go-tcp-over-google-iap/credentials"
	"github.com/nicksulia/go-tcp-over-google-iap/iap"
	"github.com/nicksulia/go-tcp-over-google-iap/logger"
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
	loglevel        string
)

var rootCmd = &cobra.Command{
	Use:   "go-tcp-over-google-iap",
	Short: "TCP tunneling over Google IAP",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		logger, err := logger.NewZapLogger(loglevel)
		if err != nil {
			logger.Fatal("Error creating logger:", err)
		}

		var creds *google.Credentials

		if credentialsFile != "" {
			creds, err = credentials.ReadCredentialsFile(ctx, credentialsFile)
		} else {
			creds, err = credentials.DefaultCredentials(ctx)
		}
		if err != nil {
			logger.Fatal("Error reading credentials file:", err)
		}

		host := iap.IAPHost{
			ProjectID: projectID,
			Zone:      zone,
			Instance:  instance,
			Interface: iface,
			Port:      port,
		}

		client, err := iap.NewIAPTunnelClient(host, creds, localPort, logger)
		if err != nil {
			logger.Fatal("Error creating IAP client", "err", err)
		}

		err = client.DryRun()
		if err != nil {
			logger.Fatal("Error during dry run", "err", err)
		}

		err = client.Serve(ctx)
		if err != nil {
			logger.Fatal("Error serving IAP tunnel", "err", err)
		}

		// Handle SIGINT/SIGTERM for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			<-sigCh
			logger.Info("Shutting down...")
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
	rootCmd.Flags().StringVar(&loglevel, "loglevel", "info", "Logging level (debug, info, warn, error)")
	rootCmd.MarkFlagRequired("project")
	rootCmd.MarkFlagRequired("zone")
	rootCmd.MarkFlagRequired("instance")

	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command:", err)
		os.Exit(1)
	}
}
