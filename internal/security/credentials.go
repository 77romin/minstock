package security

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

type CachedToken struct {
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
	KeyID     string    `json:"key_id"`
}

// SaveToken persists a short-lived broker access token in the same OS keyring
// as the API credentials. This avoids issuing a fresh token (and generating a
// broker security notification) whenever the TUI process restarts.
func SaveToken(provider, value string, expiresAt time.Time, appKey string) error {
	if strings.TrimSpace(value) == "" || expiresAt.IsZero() || strings.TrimSpace(appKey) == "" {
		return errors.New("token, expiration, and app key are required")
	}
	payload, err := json.Marshal(CachedToken{Value: value, ExpiresAt: expiresAt, KeyID: tokenKeyID(appKey)})
	if err != nil {
		return err
	}
	return keyring.Set(service, strings.ToLower(provider)+".access_token", string(payload))
}

func LoadToken(provider, appKey string) (CachedToken, error) {
	payload, err := keyring.Get(service, strings.ToLower(provider)+".access_token")
	if err != nil {
		return CachedToken{}, err
	}
	var token CachedToken
	if err := json.Unmarshal([]byte(payload), &token); err != nil {
		return CachedToken{}, err
	}
	if token.Value == "" || token.ExpiresAt.IsZero() || token.KeyID != tokenKeyID(appKey) {
		return CachedToken{}, errors.New("cached token is incomplete")
	}
	return token, nil
}

func tokenKeyID(appKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(appKey)))
	return fmt.Sprintf("%x", sum[:8])
}

func DeleteToken(provider string) {
	_ = keyring.Delete(service, strings.ToLower(provider)+".access_token")
}
