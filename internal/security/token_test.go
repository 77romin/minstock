package security

import (
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestTokenKeyringRoundTrip(t *testing.T) {
	keyring.MockInit()
	expires := time.Now().UTC().Add(12 * time.Hour).Truncate(time.Second)
	if err := SaveToken("nh", "token-value", expires, "app-key"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken("nh", "app-key")
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != "token-value" || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("unexpected cached token: %#v", got)
	}
	if _, err := LoadToken("nh", "different-app-key"); err == nil {
		t.Fatal("token was reused with a different app key")
	}
	DeleteToken("nh")
	if _, err := LoadToken("nh", "app-key"); err == nil {
		t.Fatal("token still exists after deletion")
	}
}

func TestAPIKeyKeyringRoundTrip(t *testing.T) {
	keyring.MockInit()
	if err := SaveAPIKey("alphavantage", "dividend-key"); err != nil {
		t.Fatal(err)
	}
	value, source, err := LoadAPIKey("alphavantage", "TEST_UNUSED_API_KEY")
	if err != nil || value != "dividend-key" || source != "os-keyring" {
		t.Fatalf("value=%q source=%q err=%v", value, source, err)
	}
}

func TestNewsCredentialsKeyringAndEnvironment(t *testing.T) {
	keyring.MockInit()
	t.Setenv("NAVER_CLIENT_ID", "")
	t.Setenv("NAVER_CLIENT_SECRET", "")
	t.Setenv("DART_API_KEY", "")
	if err := Save("naver", "news-id", "news-secret"); err != nil {
		t.Fatal(err)
	}
	creds, err := Load("naver")
	if err != nil || creds.AppKey != "news-id" || creds.Secret != "news-secret" || creds.Source != "os-keyring" {
		t.Fatal("NAVER keyring round trip failed")
	}
	t.Setenv("NAVER_CLIENT_ID", "env-id")
	t.Setenv("NAVER_CLIENT_SECRET", "env-secret")
	creds, err = Load("naver")
	if err != nil || creds.AppKey != "env-id" || creds.Secret != "env-secret" || creds.Source != "environment" {
		t.Fatal("NAVER environment priority failed")
	}
	if err := SaveAPIKey("dart", "dart-key"); err != nil {
		t.Fatal(err)
	}
	value, source, err := LoadAPIKey("dart", "DART_API_KEY")
	if err != nil || value != "dart-key" || source != "os-keyring" {
		t.Fatal("DART keyring round trip failed")
	}
}
