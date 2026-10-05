package kiwoom

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
)

// Opt-in read-only market-data probe. Never log credentials or raw responses.
func TestUSChartRequestProbe(t *testing.T) {
	if os.Getenv("MINSTOCK_US_CHART_PROBE") != "1" {
		t.Skip("opt-in US chart probe")
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	creds, err := security.Load("kiwoom")
	if err != nil {
		t.Fatal("Kiwoom credentials unavailable")
	}
	client, err := New(cfg.Kiwoom.BaseURL, creds)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	now := time.Now()
	for _, date := range []string{now.AddDate(-3, 0, 0).Format("20060102"), now.Format("20060102"), "20261002"} {
		body := map[string]string{"stex_tp": "ND", "stk_cd": "AAPL", "strt_dt": date, "tic_scope": "60", "upd_stkpc_tp": "1", "exrt_appl_tp": "0"}
		var out struct {
			Rows []json.RawMessage `json:"result_list"`
		}
		err := client.call(ctx, "usa06011", "/api/us/chart", body, &out)
		if err != nil {
			t.Logf("date=%s error=%v", date, err)
			continue
		}
		t.Logf("date=%s rows=%d", date, len(out.Rows))
		if len(out.Rows) > 0 {
			var row map[string]json.RawMessage
			if json.Unmarshal(out.Rows[0], &row) == nil {
				var keys []string
				for key := range row {
					keys = append(keys, key)
				}
				t.Logf("row fields=%v date=%s business_date=%s time=%s", keys, row["dt"], row["bus_dt"], row["cntr_tm"])
			} else {
				t.Log("row is not an object")
			}
		}
	}
}

func TestUSChartLiveSmoke(t *testing.T) {
	if os.Getenv("MINSTOCK_US_CHART_SMOKE") != "1" {
		t.Skip("opt-in US chart smoke")
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	creds, err := security.Load("kiwoom")
	if err != nil {
		t.Fatal("Kiwoom credentials unavailable")
	}
	client, err := New(cfg.Kiwoom.BaseURL, creds)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	for _, sample := range []struct {
		ticker   string
		interval domain.CandleInterval
	}{{"AAPL", domain.Interval60Min}, {"TSLA", domain.Interval60Min}, {"AAPL", domain.IntervalDay}} {
		now := time.Now()
		candles, err := client.Candles(ctx, domain.CandleQuery{Symbol: domain.Symbol{Code: sample.ticker, Currency: domain.USD, Market: domain.MarketUS, Exchange: "ND"}, Interval: sample.interval, From: now.AddDate(-3, 0, 0), To: now, Limit: 180, Adjusted: true})
		if err != nil || len(candles) < 120 {
			t.Fatalf("%s %s: candles=%d err=%v", sample.ticker, sample.interval, len(candles), err)
		}
		if !candles[len(candles)-1].Close.IsPositive() {
			t.Fatal("invalid candle price")
		}
		t.Logf("%s %s: %d candles, latest=%s", sample.ticker, sample.interval, len(candles), candles[len(candles)-1].OpenTime.Format(time.RFC3339))
	}
}
