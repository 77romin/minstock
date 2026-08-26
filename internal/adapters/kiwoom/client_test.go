package kiwoom

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mink/stock-min-tui/internal/domain"
	"github.com/mink/stock-min-tui/internal/security"
)

func TestInstrumentsFollowsContinuation(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_dt": "20991231235959", "return_code": 0})
			return
		}
		if r.Header.Get("authorization") != "Bearer test-token" || r.Header.Get("api-id") != "ka10099" {
			t.Fatalf("unexpected headers: %#v", r.Header)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		code := body["mrkt_tp"]
		mu.Lock()
		calls[code]++
		call := calls[code]
		mu.Unlock()
		if code == "0" && call == 1 {
			w.Header().Set("cont-yn", "Y")
			w.Header().Set("next-key", "page-2")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"list": []map[string]string{{"code": code + "01", "name": "종목" + code}}, "return_code": 0})
	}))
	defer server.Close()

	client, err := New(server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := client.Instruments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 6 {
		t.Fatalf("got %d instruments, want 6", len(symbols))
	}
	if symbols[0].Market != domain.MarketKOSPI || symbols[3].Market != domain.MarketETF {
		t.Fatalf("unexpected market mapping: %#v", symbols)
	}
}
