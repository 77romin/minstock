package kiwoom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

// SurgeQuote supplies turnover even when a candidate is outside ka10032's
// bounded top list. ka10003 accumulated turnover is already expressed in KRW.
func (c *Client) SurgeQuote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	quote, err := c.Quote(ctx, symbol)
	if err != nil {
		return quote, err
	}
	var payload struct {
		Rows []struct {
			Turnover string `json:"acc_trde_prica"`
			Power    string `json:"cntr_str"`
			Exchange string `json:"stex_tp"`
		} `json:"cntr_infr"`
	}
	if err := c.call(ctx, "ka10003", "/api/dostk/stkinfo", map[string]string{"stk_cd": symbol.Code}, &payload); err != nil {
		return quote, err
	}
	for _, row := range payload.Rows {
		if row.Exchange != "" && row.Exchange != "KRX" {
			continue
		}
		if !num(row.Turnover).IsPositive() {
			continue
		}
		quote.Turnover, quote.TradePower = num(row.Turnover), num(row.Power)
		return quote, nil
	}
	return quote, fmt.Errorf("%s KRX 누적거래대금 미제공", symbol.Code)
}

// SurgeCandidates uses KRX rankings, never the user's holdings. Ranking pages
// and detail candidates are bounded to keep polling within the shared limiter.
func (c *Client) SurgeCandidates(ctx context.Context, query domain.ScannerQuery) (domain.ScannerCandidates, error) {
	market := "000"
	if query.Market == domain.MarketKOSPI {
		market = "001"
	}
	if query.Market == domain.MarketKOSDAQ {
		market = "101"
	}
	condition, volumeCondition := "1", "1"
	if query.ExcludeETF {
		condition, volumeCondition = "16", "18"
	}
	requests := []struct {
		id, key string
		body    map[string]string
	}{
		{"ka10027", "pred_pre_flu_rt_upper", map[string]string{"mrkt_tp": market, "sort_tp": "1", "trde_qty_cnd": "0000", "stk_cnd": condition, "crd_cnd": "0", "updown_incls": "1", "pric_cnd": "0", "trde_prica_cnd": "0", "stex_tp": "1"}},
		{"ka10023", "trde_qty_sdnin", map[string]string{"mrkt_tp": market, "sort_tp": "2", "tm_tp": "1", "tm": "5", "trde_qty_tp": "5", "stk_cnd": volumeCondition, "pric_tp": "0", "stex_tp": "1"}},
		{"ka10032", "trde_prica_upper", map[string]string{"mrkt_tp": market, "mang_stk_incls": "0", "stex_tp": "1"}},
	}
	result := domain.ScannerCandidates{}
	byCode := map[string]domain.Quote{}
	var failures []error
	successful := 0
	for _, request := range requests {
		continuation, nextKey := "", ""
		for page := 0; page < 3; page++ {
			var payload map[string]json.RawMessage
			cont, next, err := c.callPage(ctx, request.id, "/api/dostk/rkinfo", request.body, &payload, continuation, nextKey)
			if err != nil {
				failures = append(failures, err)
				break
			}
			var rows []struct {
				Code     string `json:"stk_cd"`
				Name     string `json:"stk_nm"`
				Price    string `json:"cur_prc"`
				Rate     string `json:"flu_rt"`
				Turnover string `json:"trde_prica"`
				Power    string `json:"cntr_str"`
			}
			if err := json.Unmarshal(payload[request.key], &rows); err != nil {
				failures = append(failures, fmt.Errorf("%s 순위 응답: %w", request.id, err))
				break
			}
			successful++
			for _, row := range rows {
				code := strings.TrimPrefix(strings.TrimSpace(row.Code), "A")
				if len(code) != 6 {
					continue
				}
				q := byCode[code]
				// ka10032 has no ETF filter. With exclusion enabled only enrich
				// instruments already admitted by the filtered price/volume rankings.
				if query.ExcludeETF && request.id == "ka10032" && q.Symbol.Code == "" {
					continue
				}
				m := query.Market
				if m == "" {
					m = domain.MarketKRX
				}
				q.Symbol = domain.Symbol{Code: code, Ticker: code, Name: row.Name, Currency: domain.KRW, Market: m}
				q.Price, q.ChangeRate = num(row.Price), signed(row.Rate)
				// Official ka10032 trde_prica unit is one million KRW.
				if row.Turnover != "" {
					q.Turnover = num(row.Turnover).Mul(decimal.NewFromInt(1_000_000))
				}
				if row.Power != "" {
					q.TradePower = num(row.Power)
				}
				q.Provider, q.Freshness = c.ID(), domain.FreshLive
				q.ReceivedAt = time.Now()
				byCode[code] = q
			}
			if cont != "Y" {
				break
			}
			if next == "" || next == nextKey || page == 2 {
				result.Truncated = true
				break
			}
			continuation, nextKey = cont, next
		}
	}
	if successful == 0 {
		return result, errors.Join(failures...)
	}
	for _, err := range failures {
		result.Warnings = append(result.Warnings, err.Error())
	}
	for _, q := range byCode {
		result.Quotes = append(result.Quotes, q)
	}
	sort.Slice(result.Quotes, func(i, j int) bool {
		if !result.Quotes[i].ChangeRate.Equal(result.Quotes[j].ChangeRate) {
			return result.Quotes[i].ChangeRate.GreaterThan(result.Quotes[j].ChangeRate)
		}
		return result.Quotes[i].Symbol.Code < result.Quotes[j].Symbol.Code
	})
	return result, nil
}
