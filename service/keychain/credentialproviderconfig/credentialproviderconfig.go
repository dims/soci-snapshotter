package credentialproviderconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/awslabs/soci-snapshotter/service/resolver"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/log"
	"os"
	"os/exec"
	"time"
)

type options struct {
	binaryPath string
}

type Option func(*options)

func WithCredentialProviderPath(path string) Option {
	return func(opts *options) {
		opts.binaryPath = path
	}
}

func NewExternalCredentialProviderKeychain(ctx context.Context, opts ...Option) resolver.Credential {
	var kcOpts options
	for _, o := range opts {
		o(&kcOpts)
	}
	kc := &keychain{
		binaryPath: kcOpts.binaryPath,
	}
	return kc.credentials
}

type keychain struct {
	binaryPath string
}

type CredentialProviderRequest struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	Image      string `json:"image"`
}

type CredentialProviderResponse struct {
	Kind          string `json:"kind"`
	APIVersion    string `json:"apiVersion"`
	CacheKeyType  string `json:"cacheKeyType"`
	CacheDuration string `json:"cacheDuration"`
	Auth          map[string]struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auth"`
}

func (kc *keychain) credentials(imgRefSpec reference.Spec, host string) (string, string, error) {
	log.G(context.Background()).Debugf("getting creds for %s : %v ", host, imgRefSpec)
	if _, err := os.Stat(kc.binaryPath); err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("plugin binary directory %s did not exist", kc.binaryPath)
		}

		return "", "", fmt.Errorf("error inspecting binary directory %s: %w", kc.binaryPath, err)
	}
	request := &CredentialProviderRequest{
		"CredentialProviderRequest",
		"credentialprovider.kubelet.k8s.io/v1",
		imgRefSpec.String(),
	}

	data, err := json.Marshal(request)
	if err != nil {
		return "", "", fmt.Errorf("error json marshal request : %w", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := bytes.NewBuffer(data)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, kc.binaryPath)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = stdout, stderr, stdin
	err = cmd.Run()
	if err != nil {
		return "", "", fmt.Errorf("error running %s : %w", kc.binaryPath, err)
	}

	var response CredentialProviderResponse
	data = stdout.Bytes()
	err = json.Unmarshal(data, &response)
	if err != nil {
		return "", "", fmt.Errorf("error json unmarshal request: %w", err)
	}
	for _, value := range response.Auth {
		return value.Username, value.Password, nil
	}
	return "", "", fmt.Errorf("no response from %s", kc.binaryPath)
}
