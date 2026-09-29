package alphavantage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestDividendsContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("function") != "DIVIDENDS" || r.URL.Query().Get("symbol") != "QQQ" || r.URL.Query().Get("apikey") != "secret" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"symbol": "QQQ",
			"data": []map[string]string{{
				"ex_dividend_date": "2026-06-22", "declaration_date": "2026-06-19",
				"record_date": "2026-06-22", "payment_date": "2026-07-31", "amount": "0.5911",
			}},
		})
	}))
	defer server.Close()
	client, err := New(server.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	events, err := client.Dividends(t.Context(), "qqq")
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	event := events[0]
	if event.Symbol != "QQQ" || event.Currency != domain.USD || event.Freshness != domain.FreshLive || !event.Amount.Equal(decimal.RequireFromString("0.5911")) || event.PaymentDate.Format("2006-01-02") != "2026-07-31" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestDividendsReturnsProviderMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"Note": "rate limit reached"})
	}))
	defer server.Close()
	client, _ := New(server.URL, "secret")
	if _, err := client.Dividends(t.Context(), "QQQ"); err == nil {
		t.Fatal("provider error message was ignored")
	}
}
