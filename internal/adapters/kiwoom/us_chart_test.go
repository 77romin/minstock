package kiwoom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
)

func TestUSChartUsesLatestAnchorAndContinuation(t *testing.T) {
	for _, sample := range []struct{ failSecond, overlap bool }{{false, false}, {true, false}, {false, true}} {
		failSecond := sample.failSecond
		t.Run(fmt.Sprintf("second_page_failure_%v_overlap_%v", failSecond, sample.overlap), func(t *testing.T) {
			anchor := time.Date(2026, 10, 5, 0, 0, 0, 0, time.FixedZone("KST", 9*60*60))
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth2/token" {
					json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_dt": "20991231235959", "return_code": 0})
					return
				}
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if r.Header.Get("api-id") != "usa06011" || body["strt_dt"] != "20261005" || body["tic_scope"] != "60" || body["stex_tp"] != "ND" {
					t.Errorf("incorrect request: %v", body)
				}
				calls++
				if calls > 1 && (r.Header.Get("cont-yn") != "Y" || r.Header.Get("next-key") != fmt.Sprintf("page%d", calls)) {
					t.Error("continuation headers missing")
				}
				if calls == 2 && failSecond {
					json.NewEncoder(w).Encode(map[string]any{"return_code": 7, "return_msg": "second page unavailable"})
					return
				}
				var rows []map[string]string
				offset := (calls - 1) * 100
				if sample.overlap && calls > 1 {
					offset -= 32
				}
				for i := 0; i < 100; i++ {
					stamp := anchor.Add(-time.Duration(offset+i) * time.Hour)
					rows = append(rows, map[string]string{"cntr_tm": stamp.Format("20060102150405"), "bus_dt": stamp.Format("20060102"), "cur_prc": "200", "open_pric": "199", "high_pric": "201", "low_pric": "198", "trde_qty": "10"})
				}
				w.Header().Set("cont-yn", "Y")
				w.Header().Set("next-key", fmt.Sprintf("page%d", calls+1))
				json.NewEncoder(w).Encode(map[string]any{"result_list": rows, "return_code": 0})
			}))
			defer server.Close()
			client, err := New(server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			candles, err := client.Candles(t.Context(), domain.CandleQuery{Symbol: domain.Symbol{Code: "AAPL", Currency: domain.USD}, Interval: domain.Interval60Min, From: anchor.AddDate(-3, 0, 0), To: anchor, Limit: 180})
			want := 180
			if failSecond {
				want = 100
			}
			wantCalls := 2
			if sample.overlap {
				wantCalls = 3
			}
			if len(candles) != want || calls != wantCalls || (err != nil) != failSecond {
				t.Fatalf("candles=%d calls=%d err=%v", len(candles), calls, err)
			}
			if !candles[len(candles)-1].OpenTime.Equal(anchor) || !candles[0].OpenTime.Before(candles[len(candles)-1].OpenTime) {
				t.Fatal("candles not chronologically sorted")
			}
		})
	}
}

func TestUSChartRejectsInvalidTimestampsAndBoundsRepeatedKeys(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			json.NewEncoder(w).Encode(map[string]any{"token": "test-token", "expires_dt": "20991231235959", "return_code": 0})
			return
		}
		calls++
		w.Header().Set("cont-yn", "Y")
		w.Header().Set("next-key", "same-key")
		json.NewEncoder(w).Encode(map[string]any{"result_list": []map[string]string{{"cntr_tm": "invalid", "cur_prc": "200"}}, "return_code": 0})
	}))
	defer server.Close()
	client, _ := New(server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	candles, err := client.Candles(t.Context(), domain.CandleQuery{Symbol: domain.Symbol{Code: "AAPL", Currency: domain.USD}, Interval: domain.Interval60Min, Limit: 180})
	if err == nil || len(candles) != 0 || calls != 6 || !strings.Contains(err.Error(), "no valid") {
		t.Fatalf("candles=%d calls=%d err=%v", len(candles), calls, err)
	}
}
