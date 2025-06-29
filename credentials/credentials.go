package credentials

import (
	"context"
	"os"

	"golang.org/x/oauth2/google"
)

var scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
}

// ReadCredentialsFile reads Google Cloud credentials from a JSON file by absolute path and returns a Credentials object.
func ReadCredentialsFile(ctx context.Context, filename string) (*google.Credentials, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return google.CredentialsFromJSON(ctx, b, scopes...)
}

// DefaultCredentials retrieves the default Google Cloud credentials from the environment.
func DefaultCredentials(ctx context.Context) (*google.Credentials, error) {
	return google.FindDefaultCredentials(ctx, scopes...)
}
