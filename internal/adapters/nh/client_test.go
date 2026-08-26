package nh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mink/stock-min-tui/internal/security"
)

func TestAccountsContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 3600})
		case "/n2/acctinfo":
			if r.Header.Get("authorization") != "Bearer test-token" {
				t.Fatalf("authorization missing")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "rsp_msg": "완료", "Output_0": []map[string]string{{"acct_no": "1234567890", "acct_type": "01"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := client.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].ID != "1234567890" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
}
