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
