package s3presign

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	defaultSTSEndpoint   = "https://sts.amazonaws.com"
	stsAPIVersion        = "2011-06-15"
	defaultSessionName   = "hypershift-oadp"
	defaultDurationSecs  = 3600
)

// STSClient performs STS API calls using pure stdlib (no AWS SDK).
type STSClient struct {
	HTTPClient *http.Client
	Endpoint   string
}

// NewSTSClient creates a default STSClient pointing at the global STS endpoint.
func NewSTSClient() *STSClient {
	return &STSClient{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Endpoint:   defaultSTSEndpoint,
	}
}

// AssumeRoleWithWebIdentity calls the STS AssumeRoleWithWebIdentity API
// using the projected SA token on disk. This API does not require SigV4
// signing because the web identity token itself serves as authentication.
func (c *STSClient) AssumeRoleWithWebIdentity(roleARN, tokenFile, sessionName string) (*AWSCredentials, error) {
	if roleARN == "" {
		return nil, fmt.Errorf("roleARN is required")
	}
	if tokenFile == "" {
		return nil, fmt.Errorf("web identity token file path is required")
	}

	token, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read web identity token file %q: %w", tokenFile, err)
	}
	if len(token) == 0 {
		return nil, fmt.Errorf("web identity token file %q is empty", tokenFile)
	}

	if sessionName == "" {
		sessionName = defaultSessionName
	}

	params := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {stsAPIVersion},
		"RoleArn":          {roleARN},
		"RoleSessionName":  {sessionName},
		"WebIdentityToken": {string(token)},
		"DurationSeconds":  {fmt.Sprintf("%d", defaultDurationSecs)},
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = defaultSTSEndpoint
	}

	resp, err := c.HTTPClient.PostForm(endpoint, params)
	if err != nil {
		return nil, fmt.Errorf("STS request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read STS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("STS returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseSTSResponse(body)
}

// STS XML response types (subset of the full schema)
type stsResponse struct {
	XMLName xml.Name  `xml:"AssumeRoleWithWebIdentityResponse"`
	Result  stsResult `xml:"AssumeRoleWithWebIdentityResult"`
}

type stsResult struct {
	Credentials stsCredentials `xml:"Credentials"`
}

type stsCredentials struct {
	AccessKeyID     string `xml:"AccessKeyId"`
	SecretAccessKey string `xml:"SecretAccessKey"`
	SessionToken    string `xml:"SessionToken"`
}

func parseSTSResponse(data []byte) (*AWSCredentials, error) {
	var resp stsResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse STS XML response: %w", err)
	}

	creds := resp.Result.Credentials
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("STS response missing credentials")
	}

	return &AWSCredentials{
		AccessKeyID:    creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:   creds.SessionToken,
	}, nil
}
