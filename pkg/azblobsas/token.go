package azblobsas

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// TokenProvider obtains an AAD access token for a given scope.
type TokenProvider interface {
	GetToken(ctx context.Context, scope string) (string, error)
}

type aadTokenProvider struct {
	cred azcore.TokenCredential
}

// NewAADTokenProvider creates a TokenProvider that uses azidentity to acquire
// AAD tokens. It selects the credential type based on the available credentials
// and pod environment (workload identity, service principal, or managed identity).
func NewAADTokenProvider(creds *AADCredentials) (TokenProvider, error) {
	opts := &azidentity.DefaultAzureCredentialOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloudConfigForName(creds.CloudName),
		},
		TenantID: creds.TenantID,
	}

	if creds.ClientSecret != "" {
		cred, err := azidentity.NewClientSecretCredential(
			creds.TenantID, creds.ClientID, creds.ClientSecret,
			&azidentity.ClientSecretCredentialOptions{
				ClientOptions: opts.ClientOptions,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("creating client secret credential: %w", err)
		}
		return &aadTokenProvider{cred: cred}, nil
	}

	if tokenFile := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); tokenFile != "" {
		cred, err := azidentity.NewWorkloadIdentityCredential(
			&azidentity.WorkloadIdentityCredentialOptions{
				ClientOptions: opts.ClientOptions,
				TenantID:      creds.TenantID,
				ClientID:      creds.ClientID,
				TokenFilePath: tokenFile,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("creating workload identity credential: %w", err)
		}
		return &aadTokenProvider{cred: cred}, nil
	}

	cred, err := azidentity.NewManagedIdentityCredential(
		&azidentity.ManagedIdentityCredentialOptions{
			ClientOptions: opts.ClientOptions,
			ID:            azidentity.ClientID(creds.ClientID),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating managed identity credential: %w", err)
	}
	return &aadTokenProvider{cred: cred}, nil
}

func (p *aadTokenProvider) GetToken(ctx context.Context, scope string) (string, error) {
	token, err := p.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{scope},
	})
	if err != nil {
		return "", fmt.Errorf("acquiring AAD token: %w", err)
	}
	return token.Token, nil
}

// StorageScopeForCloud returns the Azure Storage OAuth2 scope for the given cloud name.
func StorageScopeForCloud(cloudName string) string {
	switch normalizeCloudName(cloudName) {
	case "azurechinacloud":
		return "https://storage.azure.cn/.default"
	default:
		return "https://storage.azure.com/.default"
	}
}

func cloudConfigForName(cloudName string) cloud.Configuration {
	switch normalizeCloudName(cloudName) {
	case "azureusgovernmentcloud", "azureusgovernment":
		return cloud.AzureGovernment
	case "azurechinacloud":
		return cloud.AzureChina
	default:
		return cloud.AzurePublic
	}
}

func normalizeCloudName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
