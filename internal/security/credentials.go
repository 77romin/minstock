package security

import (
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
		return Credentials{}, fmt.Errorf("%s credentials not configured", provider)
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
	return nil
}

func Configured(provider string) bool {
	_, err := Load(provider)
	return err == nil
}
