package kiwoom

import (
	"encoding/json"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScannerRankingsMergePagesAndNormalizeMoney(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			json.NewEncoder(w).Encode(map[string]any{"token": "test", "expires_dt": "20991231235959"})
			return
		}
		if r.URL.Path != "/api/dostk/rkinfo" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["stex_tp"] != "1" || body["mrkt_tp"] != "101" {
			t.Errorf("wrong market/session: %v", body)
		}
		switch r.Header.Get("api-id") {
		case "ka10027":
			if body["stk_cnd"] != "16" {
				t.Errorf("ETF filter missing: %v", body)
			}
			if r.Header.Get("cont-yn") == "" {
				w.Header().Set("cont-yn", "Y")
				w.Header().Set("next-key", "second")
				json.NewEncoder(w).Encode(map[string]any{"pred_pre_flu_rt_upper": []map[string]string{{"stk_cd": "A012340", "stk_nm": "시장후보", "cur_prc": "+103", "flu_rt": "+8", "cntr_str": "140"}}})
				return
			}
			if r.Header.Get("next-key") != "second" {
				t.Error("continuation missing")
			}
			json.NewEncoder(w).Encode(map[string]any{"pred_pre_flu_rt_upper": []map[string]string{{"stk_cd": "005930", "stk_nm": "두번째", "cur_prc": "100", "flu_rt": "7"}}})
		case "ka10023":
			if body["tm"] != "5" || body["stk_cnd"] != "18" {
				t.Errorf("wrong volume settings: %v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"trde_qty_sdnin": []map[string]string{{"stk_cd": "012340", "stk_nm": "시장후보", "cur_prc": "103", "flu_rt": "8"}}})
		case "ka10032":
			json.NewEncoder(w).Encode(map[string]any{"trde_prica_upper": []map[string]string{{"stk_cd": "012340", "stk_nm": "시장후보", "cur_prc": "103", "flu_rt": "8", "trde_prica": "4000"}, {"stk_cd": "999999", "stk_nm": "ETF", "cur_prc": "103", "flu_rt": "8", "trde_prica": "5000"}}})
		default:
			t.Errorf("unexpected API %s", r.Header.Get("api-id"))
		}
	}))
	defer server.Close()
	client, _ := New(server.URL, security.Credentials{AppKey: "test", Secret: "test"})
	client.limiter = rate.NewLimiter(rate.Inf, 1)
	result, err := client.SurgeCandidates(t.Context(), domain.ScannerQuery{Market: domain.MarketKOSDAQ, ExcludeETF: true})
	if err != nil || len(result.Quotes) != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	q := result.Quotes[0]
	if q.Symbol.Code != "012340" || !q.Turnover.Equal(decimal.NewFromInt(4_000_000_000)) || !q.TradePower.Equal(decimal.NewFromInt(140)) {
		t.Fatalf("incorrect normalization: %#v", q)
	}
}

func TestScannerAllFailuresAreNotEmptySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			json.NewEncoder(w).Encode(map[string]any{"token": "test", "expires_dt": "20991231235959"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"return_code": 1, "return_msg": "unavailable"})
	}))
	defer server.Close()
	client, _ := New(server.URL, security.Credentials{})
	client.limiter = rate.NewLimiter(rate.Inf, 1)
	if _, err := client.SurgeCandidates(t.Context(), domain.ScannerQuery{}); err == nil {
		t.Fatal("all failed rankings accepted")
	}
}

func TestScannerQuoteReadsKRWTurnoverWithoutMillionMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("api-id") {
		case "":
			json.NewEncoder(w).Encode(map[string]any{"token": "test", "expires_dt": "20991231235959"})
		case "ka10001":
			json.NewEncoder(w).Encode(map[string]any{"stk_cd": "012340", "cur_prc": "+103", "high_pric": "104", "flu_rt": "8"})
		case "ka10003":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["stk_cd"] != "012340" {
				t.Errorf("wrong instrument: %v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"cntr_infr": []map[string]string{{"acc_trde_prica": "9000000000", "cntr_str": "200", "stex_tp": "NXT"}, {"acc_trde_prica": "4000123456", "cntr_str": "125.25", "stex_tp": "KRX"}}})
		default:
			t.Errorf("unexpected API %s", r.Header.Get("api-id"))
		}
	}))
	defer server.Close()
	client, _ := New(server.URL, security.Credentials{})
	client.limiter = rate.NewLimiter(rate.Inf, 1)
	quote, err := client.SurgeQuote(t.Context(), domain.Symbol{Code: "012340", Currency: domain.KRW})
	if err != nil || !quote.Turnover.Equal(decimal.NewFromInt(4000123456)) || !quote.TradePower.Equal(decimal.RequireFromString("125.25")) {
		t.Fatalf("quote=%#v err=%v", quote, err)
	}
}
