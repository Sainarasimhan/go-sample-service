// Package config - GCP Specific config funcs
package config

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

func gcpDBPwd(sm string) (string, error) {
	return accessSecretVersion(sm)
}

func accessSecretVersion(name string) (string, error) {
	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create secretmanager client: %v", err)
	}

	// Build the request.
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	// Call the API.
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to access secret version: %v", err)
	}

	// WARNING: Do not print the secret in a production environment - this snippet
	// is showing how to access the secret material.
	fmt.Println("Retrieved Secret from GCP KMS")
	return string(result.Payload.Data), nil
}
