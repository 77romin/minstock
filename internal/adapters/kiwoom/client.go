package kiwoom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mink/stock-min-tui/internal/domain"
	"github.com/mink/stock-min-tui/internal/security"
	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"
)

type Client struct {
	baseURL string
	creds   security.Credentials
	http    *http.Client
	limiter *rate.Limiter
	mu      sync.Mutex
	token   string
	expires time.Time
}

func New(baseURL string, creds security.Credentials) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if err := validateBaseURL(baseURL); err != nil {
		return nil, err
	}
	return &Client{baseURL: baseURL, creds: creds, http: &http.Client{Timeout: 15 * time.Second}, limiter: rate.NewLimiter(rate.Limit(5), 1)}, nil
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("kiwoom base URL: %w", err)
	}
	allowed := u.Hostname() == "api.kiwoom.com" || u.Hostname() == "mockapi.kiwoom.com" || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if !allowed || (u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		return fmt.Errorf("refusing untrusted Kiwoom base URL %q", raw)
	}
	return nil
}

func (c *Client) ID() domain.BrokerID { return domain.BrokerKiwoom }

func (c *Client) Status(ctx context.Context) domain.BrokerStatus {
	status := domain.BrokerStatus{Broker: c.ID(), Mode: map[bool]string{true: "mock", false: "live"}[strings.Contains(c.baseURL, "mockapi")], CheckedAt: time.Now()}
	_, err := c.accessToken(ctx)
	status.Connected = err == nil
	if err != nil {
		status.Message = err.Error()
	} else {
		status.Message = "connected"
	}
	return status
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Add(10*time.Minute).Before(c.expires) {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{"grant_type": "client_credentials", "appkey": c.creds.AppKey, "secretkey": c.creds.Secret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth2/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("kiwoom token: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var out struct {
		Token   string `json:"token"`
		Expires string `json:"expires_dt"`
		Code    int    `json:"return_code"`
		Message string `json:"return_msg"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("kiwoom token response: %w", err)
	}
	if res.StatusCode >= 300 || out.Code != 0 || out.Token == "" {
		return "", fmt.Errorf("kiwoom token failed: %s", safeMessage(res.StatusCode, out.Code, out.Message))
	}
	exp, err := time.ParseInLocation("20060102150405", out.Expires, time.FixedZone("KST", 9*60*60))
	if err != nil {
		exp = time.Now().Add(23 * time.Hour)
	}
	c.token, c.expires = out.Token, exp
	return c.token, nil
}

func (c *Client) call(ctx context.Context, apiID, path string, requestBody any, out any) error {
	_, _, err := c.callPage(ctx, apiID, path, requestBody, out, "", "")
	return err
}

func (c *Client) callPage(ctx context.Context, apiID, path string, requestBody any, out any, continuation, nextKey string) (string, string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", "", err
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", "", err
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Content-Type", "application/json;charset=UTF-8")
		req.Header.Set("authorization", "Bearer "+token)
		req.Header.Set("api-id", apiID)
		if continuation != "" {
			req.Header.Set("cont-yn", continuation)
		}
		if nextKey != "" {
			req.Header.Set("next-key", nextKey)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("kiwoom %s: %w", apiID, err)
		}
		responseContinuation, responseNextKey := res.Header.Get("cont-yn"), res.Header.Get("next-key")
		b, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return "", "", readErr
		}
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.mu.Lock()
			c.token, c.expires = "", time.Time{}
			c.mu.Unlock()
			token, err = c.accessToken(ctx)
			if err != nil {
				return "", "", err
			}
			continue
		}
		var envelope struct {
			Code    *int   `json:"return_code"`
			Message string `json:"return_msg"`
		}
		_ = json.Unmarshal(b, &envelope)
		if res.StatusCode >= 300 || (envelope.Code != nil && *envelope.Code != 0) {
			code := 0
			if envelope.Code != nil {
				code = *envelope.Code
			}
			return "", "", fmt.Errorf("kiwoom %s failed: %s", apiID, safeMessage(res.StatusCode, code, envelope.Message))
		}
		if err := json.Unmarshal(b, out); err != nil {
			return "", "", fmt.Errorf("kiwoom %s decode: %w", apiID, err)
		}
		return responseContinuation, responseNextKey, nil
	}
	return "", "", fmt.Errorf("kiwoom %s authentication retry exhausted", apiID)
}

func safeMessage(status, code int, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "request failed"
	}
	return fmt.Sprintf("HTTP %d, code %d, %s", status, code, message)
}

func (c *Client) Accounts(context.Context) ([]domain.Account, error) {
	return []domain.Account{{ID: "kiwoom-default", Name: "키움 계좌", Broker: c.ID(), Currency: domain.KRW, Type: "token-bound"}}, nil
}

type balanceResponse struct {
	Purchase  string `json:"tot_pur_amt"`
	Value     string `json:"tot_evlt_amt"`
	Profit    string `json:"tot_evlt_pl"`
	Rate      string `json:"tot_prft_rt"`
	Assets    string `json:"prsm_dpst_aset_amt"`
	Positions []struct {
		Code     string `json:"stk_cd"`
		Name     string `json:"stk_nm"`
		Profit   string `json:"evltv_prft"`
		Rate     string `json:"prft_rt"`
		Average  string `json:"pur_pric"`
		Quantity string `json:"rmnd_qty"`
		Tradable string `json:"trde_able_qty"`
		Price    string `json:"cur_prc"`
		Purchase string `json:"pur_amt"`
		Value    string `json:"evlt_amt"`
	} `json:"acnt_evlt_remn_indv_tot"`
}

func (c *Client) fetchBalance(ctx context.Context) (balanceResponse, error) {
	var out balanceResponse
	err := c.call(ctx, "kt00018", "/api/dostk/acnt", map[string]string{"qry_tp": "1", "dmst_stex_tp": "KRX"}, &out)
	return out, err
}

func (c *Client) Balance(ctx context.Context, accountID string) (domain.Balance, error) {
	out, err := c.fetchBalance(ctx)
	if err != nil {
		return domain.Balance{}, err
	}
	return domain.Balance{AccountID: accountID, Broker: c.ID(), Currency: domain.KRW, PurchaseTotal: num(out.Purchase), ValueTotal: num(out.Value), ProfitLoss: signed(out.Profit), ProfitRate: signed(out.Rate), Cash: num(out.Assets).Sub(num(out.Value)), AsOf: time.Now()}, nil
}

func (c *Client) Positions(ctx context.Context, accountID string) ([]domain.Position, error) {
	out, err := c.fetchBalance(ctx)
	if err != nil {
		return nil, err
	}
	positions := make([]domain.Position, 0, len(out.Positions))
	for _, p := range out.Positions {
		code := strings.TrimPrefix(strings.TrimSpace(p.Code), "A")
		positions = append(positions, domain.Position{AccountID: accountID, Broker: c.ID(), Symbol: domain.Symbol{Code: code, Name: p.Name, Market: domain.MarketKRX, Currency: domain.KRW}, Quantity: num(p.Quantity), Tradable: num(p.Tradable), AveragePrice: num(p.Average), CurrentPrice: num(p.Price), PurchaseValue: num(p.Purchase), MarketValue: num(p.Value), ProfitLoss: signed(p.Profit), ProfitRate: signed(p.Rate), AsOf: time.Now()})
	}
	return positions, nil
}

func (c *Client) Quote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	var out struct {
		Code   string `json:"stk_cd"`
		Name   string `json:"stk_nm"`
		Price  string `json:"cur_prc"`
		Open   string `json:"open_pric"`
		High   string `json:"high_pric"`
		Low    string `json:"low_pric"`
		Change string `json:"pred_pre"`
		Rate   string `json:"flu_rt"`
		Volume string `json:"trde_qty"`
	}
	if err := c.call(ctx, "ka10001", "/api/dostk/stkinfo", map[string]string{"stk_cd": symbol.Code}, &out); err != nil {
		return domain.Quote{}, err
	}
	if symbol.Name == "" {
		symbol.Name = out.Name
	}
	now := time.Now()
	return domain.Quote{Symbol: symbol, Price: num(out.Price), Open: num(out.Open), High: num(out.High), Low: num(out.Low), Change: signed(out.Change), ChangeRate: signed(out.Rate), Volume: intNum(out.Volume), MarketTime: now, ReceivedAt: now, Provider: c.ID(), Freshness: domain.FreshLive}, nil
}

// USDKRW reads the base exchange rate included in Kiwoom's official US-stock quote.
// The endpoint does not expose a previous-day comparison, so Change fields remain zero.
func (c *Client) USDKRW(ctx context.Context) (domain.FXRate, error) {
	var out struct {
		Rate string `json:"base_exrt"`
	}
	if err := c.call(ctx, "usa20100", "/api/us/mrkcond", map[string]string{"stex_tp": "ND", "stk_cd": "NVDA"}, &out); err != nil {
		return domain.FXRate{}, err
	}
	rate := num(out.Rate)
	if !rate.IsPositive() {
		return domain.FXRate{}, fmt.Errorf("kiwoom USD/KRW response did not include base_exrt")
	}
	return domain.FXRate{Base: domain.USD, Quote: domain.KRW, Rate: rate, AsOf: time.Now(), Provider: c.ID(), Freshness: domain.FreshLive}, nil
}

func (c *Client) Instruments(ctx context.Context) ([]domain.Symbol, error) {
	markets := []struct {
		code   string
		market domain.Market
	}{{"0", domain.MarketKOSPI}, {"10", domain.MarketKOSDAQ}, {"8", domain.MarketETF}, {"50", domain.MarketKONEX}, {"30", domain.MarketKOTC}}
	var result []domain.Symbol
	for _, m := range markets {
		continuation, nextKey := "", ""
		for {
			var out struct {
				List []struct {
					Code       string `json:"code"`
					Name       string `json:"name"`
					MarketName string `json:"marketName"`
				} `json:"list"`
			}
			var err error
			continuation, nextKey, err = c.callPage(ctx, "ka10099", "/api/dostk/stkinfo", map[string]string{"mrkt_tp": m.code}, &out, continuation, nextKey)
			if err != nil {
				return nil, err
			}
			for _, s := range out.List {
				result = append(result, domain.Symbol{Code: s.Code, Name: s.Name, Market: m.market, Currency: domain.KRW})
			}
			if !strings.EqualFold(continuation, "Y") || nextKey == "" {
				break
			}
		}
	}
	return result, nil
}

func (c *Client) WatchlistGroups(ctx context.Context) ([]domain.WatchlistGroup, error) {
	var out struct {
		Groups []struct {
			Code string `json:"gcod"`
			Name string `json:"name"`
		} `json:"nofi"`
	}
	if err := c.call(ctx, "ka01300", "/api/dostk/watchlist", map[string]string{}, &out); err != nil {
		return nil, err
	}
	groups := make([]domain.WatchlistGroup, 0, len(out.Groups))
	for _, g := range out.Groups {
		groups = append(groups, domain.WatchlistGroup{ID: g.Code, ExternalID: g.Code, Name: g.Name, Provider: c.ID()})
	}
	return groups, nil
}

func (c *Client) WatchlistItems(ctx context.Context, groupID string) ([]domain.WatchlistItem, error) {
	var out struct {
		Items []struct {
			Code string `json:"cod2"`
		} `json:"nofj"`
	}
	if err := c.call(ctx, "ka01301", "/api/dostk/watchlist", map[string]string{"arn_grp_id": groupID}, &out); err != nil {
		return nil, err
	}
	items := make([]domain.WatchlistItem, 0, len(out.Items))
	for _, i := range out.Items {
		items = append(items, domain.WatchlistItem{GroupID: groupID, Provider: c.ID(), Symbol: domain.Symbol{Code: i.Code, Market: domain.MarketKRX, Currency: domain.KRW}})
	}
	return items, nil
}

func (c *Client) Candles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	apiID, pathKey := "ka10081", "stk_dt_pole_chart_qry"
	body := map[string]string{"stk_cd": q.Symbol.Code, "base_dt": q.To.Format("20060102"), "upd_stkpc_tp": map[bool]string{true: "1", false: "0"}[q.Adjusted]}
	switch q.Interval {
	case domain.IntervalTick:
		apiID, pathKey = "ka10079", "stk_tic_pole_chart_qry"
		body["tic_scope"] = "1"
		delete(body, "base_dt")
	case domain.Interval1Min, domain.Interval5Min, domain.Interval15Min, domain.Interval60Min:
		apiID, pathKey = "ka10080", "stk_min_pole_chart_qry"
		body["tic_scope"] = strings.TrimSuffix(string(q.Interval), "m")
	case domain.IntervalWeek:
		apiID, pathKey = "ka10082", "stk_stk_pole_chart_qry"
	case domain.IntervalMonth:
		apiID, pathKey = "ka10083", "stk_mth_pole_chart_qry"
	case domain.IntervalYear:
		apiID, pathKey = "ka10094", "stk_yr_pole_chart_qry"
	}
	var raw map[string]json.RawMessage
	if err := c.call(ctx, apiID, "/api/dostk/chart", body, &raw); err != nil {
		return nil, err
	}
	var rows []struct {
		Close    string `json:"cur_prc"`
		Volume   string `json:"trde_qty"`
		Turnover string `json:"trde_prica"`
		Date     string `json:"dt"`
		Time     string `json:"cntr_tm"`
		Open     string `json:"open_pric"`
		High     string `json:"high_pric"`
		Low      string `json:"low_pric"`
	}
	if err := json.Unmarshal(raw[pathKey], &rows); err != nil {
		return nil, fmt.Errorf("kiwoom %s candles: %w", apiID, err)
	}
	limit := q.Limit
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	rows = rows[:limit]
	loc := time.FixedZone("KST", 9*60*60)
	candles := make([]domain.Candle, 0, len(rows))
	for _, row := range rows {
		stamp := row.Date
		layout := "20060102"
		if row.Time != "" {
			stamp = row.Time
			layout = "20060102150405"
		}
		t, err := time.ParseInLocation(layout, stamp, loc)
		if err != nil {
			continue
		}
		candles = append(candles, domain.Candle{Symbol: q.Symbol, Interval: q.Interval, OpenTime: t, CloseTime: closeTime(t, q.Interval), Open: num(row.Open), High: num(row.High), Low: num(row.Low), Close: num(row.Close), Volume: intNum(row.Volume), Turnover: num(row.Turnover), Adjusted: q.Adjusted, Complete: true, Provider: c.ID()})
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime.Before(candles[j].OpenTime) })
	return candles, nil
}

func closeTime(t time.Time, i domain.CandleInterval) time.Time {
	switch i {
	case domain.Interval1Min:
		return t.Add(time.Minute)
	case domain.Interval5Min:
		return t.Add(5 * time.Minute)
	case domain.Interval15Min:
		return t.Add(15 * time.Minute)
	case domain.Interval60Min:
		return t.Add(time.Hour)
	case domain.IntervalWeek:
		return t.AddDate(0, 0, 7)
	case domain.IntervalMonth:
		return t.AddDate(0, 1, 0)
	case domain.IntervalYear:
		return t.AddDate(1, 0, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}
func signed(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(s, ",", "")))
	return d
}
func num(s string) decimal.Decimal { return signed(s).Abs() }
func intNum(s string) int64 {
	clean := strings.TrimLeft(strings.TrimSpace(strings.ReplaceAll(s, ",", "")), "+")
	v, _ := strconv.ParseInt(clean, 10, 64)
	if v < 0 {
		return -v
	}
	return v
}
