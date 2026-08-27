package security

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const service = "minstock"

type Credentials struct {
	AppKey string
	Secret string
	Source string
}

func Load(provider string) (Credentials, error) {
	provider = strings.ToLower(provider)
	prefix := strings.ToUpper(provider)
	if provider == "nh" {
		prefix = "NHPLUG"
	}
	appKey, secret := os.Getenv(prefix+"_APP_KEY"), os.Getenv(prefix+"_APP_SECRET")
	if appKey != "" && secret != "" {
		return Credentials{AppKey: appKey, Secret: secret, Source: "environment"}, nil
	}
	appKey, keyErr := keyring.Get(service, provider+".app_key")
	secret, secretErr := keyring.Get(service, provider+".app_secret")
	if keyErr != nil || secretErr != nil || appKey == "" || secret == "" {
		if (keyErr != nil && !errors.Is(keyErr, keyring.ErrNotFound)) || (secretErr != nil && !errors.Is(secretErr, keyring.ErrNotFound)) {
			backendErr := keyErr
			if backendErr == nil {
				backendErr = secretErr
			}
			return Credentials{}, fmt.Errorf("%s OS keyring unavailable or access denied: %v", provider, backendErr)
		}
		return Credentials{}, fmt.Errorf("%s credentials not found in environment or OS keyring", provider)
	}
	return Credentials{AppKey: appKey, Secret: secret, Source: "os-keyring"}, nil
}

func Save(provider, appKey, secret string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "kiwoom" && provider != "nh" {
		return fmt.Errorf("unsupported provider %q", provider)
	}
	if strings.TrimSpace(appKey) == "" || strings.TrimSpace(secret) == "" {
		return fmt.Errorf("app key and secret are required")
	}
	if err := keyring.Set(service, provider+".app_key", strings.TrimSpace(appKey)); err != nil {
		return fmt.Errorf("save app key: %w", err)
	}
	if err := keyring.Set(service, provider+".app_secret", strings.TrimSpace(secret)); err != nil {
		return fmt.Errorf("save app secret: %w", err)
	}
	// Verify through the keyring backend itself (rather than Load, which may
	// prefer environment variables) so setup never reports success for a
	// value that was not persisted by macOS/Linux secret storage.
	storedKey, err := keyring.Get(service, provider+".app_key")
	if err != nil || storedKey != strings.TrimSpace(appKey) {
		return fmt.Errorf("verify app key in OS keyring: value was not persisted")
	}
	storedSecret, err := keyring.Get(service, provider+".app_secret")
	if err != nil || storedSecret != strings.TrimSpace(secret) {
		return fmt.Errorf("verify app secret in OS keyring: value was not persisted")
	}
	return nil
}

func Configured(provider string) bool {
	_, err := Load(provider)
	return err == nil
}
