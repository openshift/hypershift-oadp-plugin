package azblobsas

import (
	"testing"
)

func TestStorageScopeForCloud(t *testing.T) {
	tests := []struct {
		cloud string
		want  string
	}{
		{"AzurePublicCloud", "https://storage.azure.com/.default"},
		{"AzureCloud", "https://storage.azure.com/.default"},
		{"", "https://storage.azure.com/.default"},
		{"AzureUSGovernmentCloud", "https://storage.azure.com/.default"},
		{"AzureUSGovernment", "https://storage.azure.com/.default"},
		{"AzureChinaCloud", "https://storage.azure.cn/.default"},
	}

	for _, tt := range tests {
		t.Run(tt.cloud, func(t *testing.T) {
			got := StorageScopeForCloud(tt.cloud)
			if got != tt.want {
				t.Errorf("StorageScopeForCloud(%q) = %q, want %q", tt.cloud, got, tt.want)
			}
		})
	}
}

func TestCloudConfigForName(t *testing.T) {
	tests := []struct {
		cloud         string
		wantAuthority string
	}{
		{"AzurePublicCloud", "https://login.microsoftonline.com/"},
		{"AzureCloud", "https://login.microsoftonline.com/"},
		{"", "https://login.microsoftonline.com/"},
		{"AzureUSGovernmentCloud", "https://login.microsoftonline.us/"},
		{"AzureUSGovernment", "https://login.microsoftonline.us/"},
		{"AzureChinaCloud", "https://login.chinacloudapi.cn/"},
	}

	for _, tt := range tests {
		t.Run(tt.cloud, func(t *testing.T) {
			cfg := cloudConfigForName(tt.cloud)
			got := cfg.ActiveDirectoryAuthorityHost
			if got != tt.wantAuthority {
				t.Errorf("cloudConfigForName(%q).ActiveDirectoryAuthorityHost = %q, want %q", tt.cloud, got, tt.wantAuthority)
			}
		})
	}
}
