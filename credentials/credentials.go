package credentials

import (
	"context"
	"os"

	"golang.org/x/oauth2/google"
)

var scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
}

func ReadCredentialsFile(ctx context.Context, filename string) (*google.Credentials, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return google.CredentialsFromJSON(ctx, b, scopes...)
}

func DefaultCredentials(ctx context.Context) (*google.Credentials, error) {
	return google.FindDefaultCredentials(ctx, scopes...)
}
