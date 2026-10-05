package alphavantage

import (
	"encoding/json"
	"github.com/77romin/minstock-tui/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewsUsesTickerAndFiltersUnrelatedResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("function") != "NEWS_SENTIMENT" || q.Get("tickers") != "AAPL" || q.Get("apikey") != "test-key" {
			t.Error("wrong news request")
		}
		json.NewEncoder(w).Encode(map[string]any{"feed": []map[string]any{
			{"title": "Apple earnings", "url": "https://press.example/a", "source": "News", "time_published": "20261005T010203", "ticker_sentiment": []map[string]string{{"ticker": "AAPL", "relevance_score": "0.85"}}},
			{"title": "Unrelated", "url": "https://press.example/b", "source": "News", "time_published": "20261005T010203", "ticker_sentiment": []map[string]string{{"ticker": "NVDA", "relevance_score": "0.9"}}},
		}})
	}))
	defer server.Close()
	client, _ := New(server.URL, "test-key")
	items, err := client.Information(t.Context(), domain.Symbol{Code: "aapl", Currency: domain.USD})
	if err != nil || len(items) != 1 || items[0].Relevance != "0.85" || items[0].PublishedAt.Hour() != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestNewsQuotaAndMissingFeedAreErrors(t *testing.T) {
	for _, body := range []string{`{"Information":"25 requests rate limit test-key"}`, `{"Note":"API key test-key invalid"}`, `{}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			client, _ := New(server.URL, "test-key")
			_, err := client.Information(t.Context(), domain.Symbol{Code: "AAPL"})
			if err == nil || strings.Contains(err.Error(), "test-key") {
				t.Fatalf("incorrect or unredacted error: %v", err)
			}
		})
	}
}
