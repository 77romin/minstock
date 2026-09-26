package kiwoom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/77romin/stock-min-tui/internal/domain"
	"github.com/77romin/stock-min-tui/internal/security"
	"github.com/shopspring/decimal"
)

func TestInstrumentsFollowsContinuation(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_dt": "20991231235959", "return_code": 0})
			return
		}
		if r.Header.Get("authorization") != "Bearer test-token" || (r.Header.Get("api-id") != "ka10099" && r.Header.Get("api-id") != "usa10099") {
			t.Fatalf("unexpected headers: %#v", r.Header)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		code := body["mrkt_tp"]
		if r.Header.Get("api-id") == "usa10099" {
			_ = json.NewEncoder(w).Encode(map[string]any{"us_stklist": []map[string]string{{"stk_cd": body["stex_tp"] + "-AAPL", "stk_nm": "Apple"}}, "return_code": 0})
			return
		}
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
	if len(symbols) != 9 {
		t.Fatalf("got %d instruments, want 9", len(symbols))
	}
	if symbols[0].Market != domain.MarketKOSPI || symbols[3].Market != domain.MarketETF {
		t.Fatalf("unexpected market mapping: %#v", symbols)
	}
	if symbols[len(symbols)-1].Market != domain.MarketUS || symbols[len(symbols)-1].Ticker == "" {
		t.Fatalf("US instruments were not mapped: %#v", symbols[len(symbols)-3:])
	}
}

func TestParseUSWatchlistPayloads(t *testing.T) {
	groups := parseUSWatchlistGroups(json.RawMessage(`{"us_wtch_grp":[{"grp_no":"7","grp_nm":"미국 성장주"}]}`))
	if len(groups) != 1 || groups[0].Code != "7" || groups[0].Name != "미국 성장주" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	items := parseUSWatchlistItems(json.RawMessage(`{"us_wtch_stk":[{"stk_cd":"AAPL","stk_nm":"Apple","stex_tp":"ND"}]}`))
	if len(items) != 1 || items[0].Code != "AAPL" || items[0].Name != "Apple" || items[0].Exchange != "ND" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestUSPortfolioQuoteAndCandles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_dt": "20991231235959", "return_code": 0})
			return
		}
		switch r.Header.Get("api-id") {
		case "ust21070":
			_ = json.NewEncoder(w).Encode(map[string]any{"crnc_code": "USD", "tot_evlt_amt": "4588.20", "tot_prch_amt": "4200.50", "tot_pl_amt": "387.70", "tot_pl_rt": "9.23", "result_list": []map[string]string{{"stex_nm": "NASDAQ", "crnc_code": "USD", "stk_cd": "AAPL", "frgn_stk_nm": "Apple", "poss_qty": "20", "sell_alowq": "20", "frgn_stk_book_uv": "210.025", "now_pric": "229.41", "evlt_amt": "4588.20", "pl_amt": "387.70", "pl_rt": "9.23", "frgn_stk_book_amt": "4200.50"}}, "return_code": 0})
		case "ust21110":
			_ = json.NewEncoder(w).Encode(map[string]any{"result_list": []map[string]string{{"crnc_code": "USD", "fc_entra": "812.45"}}, "return_code": 0})
		case "usa20100":
			_ = json.NewEncoder(w).Encode(map[string]any{"stex_tp": "ND", "stk_cd": "AAPL", "stk_nm": "Apple", "cur_prc": "229.41", "base_close_pric": "225.00", "pred_pre": "+4.41", "flu_rt": "+1.96", "acc_trde_qty": "10000", "pre_open_pric": "226.00", "pre_high_pric": "230.00", "pre_low_pric": "224.00", "return_code": 0})
		case "usa06012":
			_ = json.NewEncoder(w).Encode(map[string]any{"result_list": []map[string]string{{"cur_prc": "229.41", "acc_trde_qty": "10000", "acc_trde_prica": "2294100", "open_pric": "226.00", "high_pric": "230.00", "low_pric": "224.00", "dt": "20260826"}}, "return_code": 0})
		default:
			t.Fatalf("unexpected API ID %q", r.Header.Get("api-id"))
		}
	}))
	defer server.Close()
	client, err := New(server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	balance, err := client.Balance(t.Context(), accountUS)
	if err != nil || !balance.Cash.Equal(decimal.RequireFromString("812.45")) || balance.Currency != domain.USD {
		t.Fatalf("US balance: %#v %v", balance, err)
	}
	positions, err := client.Positions(t.Context(), accountUS)
	if err != nil || len(positions) != 1 || positions[0].Symbol.Exchange != "ND" {
		t.Fatalf("US positions: %#v %v", positions, err)
	}
	quote, err := client.Quote(t.Context(), positions[0].Symbol)
	if err != nil || !quote.Price.Equal(decimal.RequireFromString("229.41")) {
		t.Fatalf("US quote: %#v %v", quote, err)
	}
	candles, err := client.Candles(t.Context(), domain.CandleQuery{Symbol: positions[0].Symbol, Interval: domain.IntervalDay, From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Now(), Limit: 10, Adjusted: true})
	if err != nil || len(candles) != 1 || candles[0].Symbol.Currency != domain.USD {
		t.Fatalf("US candles: %#v %v", candles, err)
	}
}

func TestOrderAPIIsRejectedBeforeNetwork(t *testing.T) {
	client, err := New("http://127.0.0.1:1", security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	err = client.call(context.Background(), "order-api", "/api/us/ordr", map[string]string{}, &map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "read-only allowlist") {
		t.Fatalf("order API was not rejected: %v", err)
	}
}
