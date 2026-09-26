package nh

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

	"github.com/77romin/stock-min-tui/internal/domain"
	"github.com/77romin/stock-min-tui/internal/security"
	"github.com/shopspring/decimal"
	"golang.org/x/time/rate"
)

type Client struct {
	baseURL    string
	authURL    string
	creds      security.Credentials
	http       *http.Client
	limiter    *rate.Limiter
	mu         sync.Mutex
	token      string
	expires    time.Time
	balanceMu  sync.Mutex
	balances   map[string]cachedBalance
	cacheToken bool
}

type cachedBalance struct {
	value     balanceEnvelope
	fetchedAt time.Time
}

func New(baseURL, authURL string, creds security.Credentials) (*Client, error) {
	baseURL, authURL = strings.TrimRight(baseURL, "/"), strings.TrimRight(authURL, "/")
	if err := validateURL(baseURL); err != nil {
		return nil, err
	}
	if err := validateURL(authURL); err != nil {
		return nil, err
	}
	auth, _ := url.Parse(authURL)
	cacheToken := auth.Hostname() != "localhost" && auth.Hostname() != "127.0.0.1"
	return &Client{
		baseURL: baseURL, authURL: authURL, creds: creds,
		http: &http.Client{Timeout: 15 * time.Second}, limiter: rate.NewLimiter(4, 1),
		balances: make(map[string]cachedBalance), cacheToken: cacheToken,
	}, nil
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	h := u.Hostname()
	allowed := h == "api.nhplug.com" || h == "moapi.nhplug.com" || h == "api.n2plug.com" || h == "moapi.n2plug.com" || h == "localhost" || h == "127.0.0.1"
	if !allowed || (u.Scheme != "https" && h != "localhost" && h != "127.0.0.1") {
		return fmt.Errorf("refusing untrusted NH base URL %q", raw)
	}
	return nil
}
func (c *Client) ID() domain.BrokerID { return domain.BrokerNH }
func (c *Client) Status(ctx context.Context) domain.BrokerStatus {
	s := domain.BrokerStatus{Broker: c.ID(), Mode: map[bool]string{true: "mock", false: "live"}[strings.Contains(c.baseURL, "moapi")], CheckedAt: time.Now()}
	_, err := c.accessToken(ctx)
	s.Connected = err == nil
	if err != nil {
		s.Message = err.Error()
	} else {
		s.Message = "connected"
	}
	return s
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Add(time.Minute).Before(c.expires) {
		return c.token, nil
	}
	if c.cacheToken {
		if cached, err := security.LoadToken("nh", c.creds.AppKey); err == nil && time.Now().Add(time.Minute).Before(cached.ExpiresAt) {
			c.token, c.expires = cached.Value, cached.ExpiresAt
			return c.token, nil
		}
	}
	values := url.Values{"appkey": {c.creds.AppKey}, "appsecretkey": {c.creds.Secret}, "grant_type": {"client_credentials"}, "scope": {"oob"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authURL+"/oauth2/token?"+values.Encode(), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("nh token: %w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var out struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
		Message string `json:"rsp_msg"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("nh token response: %w", err)
	}
	if res.StatusCode >= 300 || out.Token == "" {
		return "", fmt.Errorf("nh token failed: HTTP %d %s", res.StatusCode, out.Message)
	}
	if out.Expires <= 0 {
		out.Expires = 86400
	}
	c.token = out.Token
	c.expires = time.Now().Add(time.Duration(out.Expires) * time.Second)
	// Authentication must still succeed if the local keyring is temporarily
	// unavailable. In that case this process keeps using its in-memory token.
	if c.cacheToken {
		_ = security.SaveToken("nh", c.token, c.expires, c.creds.AppKey)
	}
	return c.token, nil
}

func (c *Client) call(ctx context.Context, path string, input map[string]any, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"Input_0": input})
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
		req.Header.Set("x-client-id", c.creds.AppKey)
		req.Header.Set("x-client-secret", c.creds.Secret)
		req.Header.Set("authorization", "Bearer "+token)
		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("nh %s: %w", path, err)
		}
		b, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.mu.Lock()
			c.token = ""
			c.expires = time.Time{}
			c.mu.Unlock()
			if c.cacheToken {
				security.DeleteToken("nh")
			}
			token, err = c.accessToken(ctx)
			if err != nil {
				return err
			}
			continue
		}
		var envelope struct {
			Code    string `json:"rsp_cd"`
			Message string `json:"rsp_msg"`
		}
		_ = json.Unmarshal(b, &envelope)
		if res.StatusCode >= 300 || !nhSuccess(envelope.Code, envelope.Message) {
			return fmt.Errorf("nh %s failed: HTTP %d %s %s", path, res.StatusCode, envelope.Code, envelope.Message)
		}
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("nh %s decode: %w", path, err)
		}
		return nil
	}
	return fmt.Errorf("nh %s authentication retry exhausted", path)
}

func nhSuccess(code, message string) bool {
	if code == "" || code == "00000" || code == "00166" || code == "00221" || code == "13578" {
		return true
	}
	return strings.Contains(message, "완료")
}

func (c *Client) Accounts(ctx context.Context) ([]domain.Account, error) {
	var out struct {
		Rows []map[string]any `json:"Output_0"`
	}
	if err := c.call(ctx, "/n2/acctinfo", map[string]any{}, &out); err != nil {
		return nil, err
	}
	accounts := make([]domain.Account, 0, len(out.Rows))
	mock := strings.Contains(c.baseURL, "moapi")
	for _, row := range out.Rows {
		id := str(row, "acct_no")
		typ := str(row, "acct_type")
		usable := (mock && typ == "03") || (!mock && (typ == "01" || typ == "02"))
		if usable {
			accounts = append(accounts, domain.Account{ID: id, Name: "NH " + maskAccount(id), Broker: c.ID(), Currency: domain.KRW, Type: typ})
		}
	}
	return accounts, nil
}

type balanceEnvelope struct {
	Summary   map[string]any   `json:"Output_0"`
	Positions []map[string]any `json:"Output_1"`
}

func (c *Client) fetchBalance(ctx context.Context, accountID string) (balanceEnvelope, error) {
	c.balanceMu.Lock()
	defer c.balanceMu.Unlock()
	if cached, ok := c.balances[accountID]; ok && time.Since(cached.fetchedAt) < 5*time.Second {
		return cached.value, nil
	}
	var out balanceEnvelope
	err := c.call(ctx, "/krstock/inquiry/v1/balance", map[string]any{
		"act_no": accountID, "bnc_bse_cd": "5", "ltg_aot_dit_cd": "9", "aet_bse": "2",
		"qut_dit_cd": "UNT", "aly_qut_cd": "1",
	}, &out)
	if err == nil {
		c.balances[accountID] = cachedBalance{value: out, fetchedAt: time.Now()}
	}
	return out, err
}
func (c *Client) Balance(ctx context.Context, accountID string) (domain.Balance, error) {
	out, err := c.fetchBalance(ctx, accountID)
	if err != nil {
		return domain.Balance{}, err
	}
	s := out.Summary
	value := decAny(s, "tot_aet_amt", "evlu_amt", "tot_evlu_amt")
	cash := decAny(s, "dca", "dnca_tot_amt")
	purchase := decAny(s, "pchs_amt_smtl_amt", "tot_pchs_amt")
	profit := decAnySigned(s, "evlu_pfls_smtl_amt", "tot_evlu_pfls_amt")
	rate := decimal.Zero
	if purchase.IsPositive() {
		rate = profit.Div(purchase).Mul(decimal.NewFromInt(100))
	}
	return domain.Balance{AccountID: accountID, Broker: c.ID(), Currency: domain.KRW, Cash: cash, PurchaseTotal: purchase, ValueTotal: value, ProfitLoss: profit, ProfitRate: rate, AsOf: time.Now()}, nil
}
func (c *Client) Positions(ctx context.Context, accountID string) ([]domain.Position, error) {
	out, err := c.fetchBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}
	positions := make([]domain.Position, 0, len(out.Positions))
	for _, p := range out.Positions {
		code := first(p, "iem_cd", "pdno", "stck_shrn_iscd")
		symbol := domain.Symbol{Code: code, Ticker: code, Name: first(p, "iem_nm", "prdt_name"), Market: domain.MarketKRX, Currency: domain.KRW}
		positions = append(positions, domain.Position{AccountID: accountID, Broker: c.ID(), Symbol: symbol, Quantity: decAny(p, "hldg_qty", "hold_qty"), Tradable: decAny(p, "ord_psbl_qty", "sell_psbl_qty"), AveragePrice: decAny(p, "pchs_avg_pric", "pchs_avg_prc"), CurrentPrice: decAny(p, "stck_prpr", "prpr"), PurchaseValue: decAny(p, "pchs_amt", "buy_amt"), MarketValue: decAny(p, "evlu_amt", "evlu_pfls_amt"), ProfitLoss: decAnySigned(p, "evlu_pfls_amt", "evlu_pfls"), ProfitRate: decAnySigned(p, "evlu_pfls_rt", "evlu_erng_rt"), AsOf: time.Now()})
	}
	foreign, foreignErr := c.foreignPositions(ctx, accountID)
	if foreignErr != nil {
		if len(positions) == 0 {
			return nil, foreignErr
		}
		return positions, nil
	}
	positions = append(positions, foreign...)
	return positions, nil
}

func (c *Client) foreignPositions(ctx context.Context, accountID string) ([]domain.Position, error) {
	var out struct {
		Positions []map[string]any `json:"Output_1"`
	}
	if err := c.call(ctx, "/gbstock/inquiry/v1/balance", map[string]any{
		"act_no": accountID, "qut_iqr_dit_cd": "9", "fc_sec_trd_nat_cd": "200",
		"cur_cd": "USD", "xns_dit_cd": "1",
	}, &out); err != nil {
		return nil, err
	}
	positions := make([]domain.Position, 0, len(out.Positions))
	for _, p := range out.Positions {
		code := first(p, "iem_cd")
		if code == "" {
			continue
		}
		name := first(p, "iem_nm", "oss_iem_eng_nm")
		symbol := domain.Symbol{Code: code, Ticker: code, Name: name, Market: domain.MarketUS, Currency: domain.USD}
		positions = append(positions, domain.Position{
			AccountID: accountID, Broker: c.ID(), Symbol: symbol,
			Quantity: decAny(p, "cns_bse_bnc_qty"), Tradable: decAny(p, "sll_pbl_qty1"),
			AveragePrice: decAny(p, "fc_avg_phs_pr", "fc_phs_uit_pr"), CurrentPrice: decAny(p, "fc_sec_end_pr"),
			PurchaseValue: decAny(p, "fc_abk_amt", "fc_cns_bse_phs_xps"), MarketValue: decAny(p, "fc_eal_amt"),
			ProfitLoss: decAnySigned(p, "fc_eal_pls_amt"), ProfitRate: decAnySigned(p, "eal_pft_rt", "eal_pft_rt1"),
			AsOf: time.Now(),
		})
	}
	return positions, nil
}

func (c *Client) Quote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	var out struct {
		Row map[string]any `json:"Output_0"`
	}
	if err := c.call(ctx, "/krstock/quote/v1/currentPrice", map[string]any{"iem_cd": symbol.Code, "market_cd": "KRX"}, &out); err != nil {
		return domain.Quote{}, err
	}
	if symbol.Name == "" {
		symbol.Name = first(out.Row, "iem_nm", "prdt_name")
	}
	now := time.Now()
	return domain.Quote{Symbol: symbol, Price: decAny(out.Row, "stck_prpr"), Open: decAny(out.Row, "stck_oprc"), High: decAny(out.Row, "stck_hgpr"), Low: decAny(out.Row, "stck_lwpr"), Change: decAnySigned(out.Row, "prdy_vrss"), ChangeRate: decAnySigned(out.Row, "prdy_ctrt"), Volume: intAny(out.Row, "acml_vol"), Turnover: decAny(out.Row, "acml_tr_pbmn"), TradePower: decAny(out.Row, "tday_rltv"), MarketTime: now, ReceivedAt: now, Provider: c.ID(), Freshness: domain.FreshLive}, nil
}

func (c *Client) Candles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	if q.Interval == domain.IntervalTick || q.Interval == domain.Interval1Min || q.Interval == domain.Interval5Min || q.Interval == domain.Interval15Min || q.Interval == domain.Interval60Min {
		return nil, fmt.Errorf("NH domestic intraday candles are unavailable; configure Kiwoom for this interval")
	}
	count := q.Limit
	if count <= 0 {
		count = 300
	}
	if count > 1000 {
		count = 1000
	}
	var out struct {
		Rows []map[string]any `json:"Output_0"`
	}
	if err := c.call(ctx, "/krstock/quote/v1/currentDaily", map[string]any{"market_cd": "KRX", "iem_cd": q.Symbol.Code, "array_cnt": strconv.Itoa(count)}, &out); err != nil {
		return nil, err
	}
	loc := time.FixedZone("KST", 9*60*60)
	daily := make([]domain.Candle, 0, len(out.Rows))
	for _, row := range out.Rows {
		t, err := time.ParseInLocation("20060102", first(row, "bsop_date"), loc)
		if err != nil {
			continue
		}
		daily = append(daily, domain.Candle{Symbol: q.Symbol, Interval: domain.IntervalDay, OpenTime: t, CloseTime: t.AddDate(0, 0, 1), Open: decAny(row, "stck_oprc"), High: decAny(row, "stck_hgpr"), Low: decAny(row, "stck_lwpr"), Close: decAny(row, "stck_clpr"), Volume: intAny(row, "acml_vol", "acml_voln"), Turnover: decAny(row, "acml_tr_pbmn"), Adjusted: q.Adjusted, Complete: true, Provider: c.ID()})
	}
	sort.Slice(daily, func(i, j int) bool { return daily[i].OpenTime.Before(daily[j].OpenTime) })
	if q.Interval == domain.IntervalDay {
		return daily, nil
	}
	return aggregateCalendar(daily, q.Interval), nil
}

func aggregateCalendar(input []domain.Candle, target domain.CandleInterval) []domain.Candle {
	var out []domain.Candle
	keyFor := func(t time.Time) string {
		switch target {
		case domain.IntervalWeek:
			y, w := t.ISOWeek()
			return fmt.Sprintf("%04d-%02d", y, w)
		case domain.IntervalMonth:
			return t.Format("2006-01")
		default:
			return t.Format("2006")
		}
	}
	last := ""
	for _, source := range input {
		k := keyFor(source.OpenTime)
		if k != last {
			c := source
			c.Interval = target
			out = append(out, c)
			last = k
			continue
		}
		c := &out[len(out)-1]
		if source.High.GreaterThan(c.High) {
			c.High = source.High
		}
		if source.Low.LessThan(c.Low) {
			c.Low = source.Low
		}
		c.Close = source.Close
		c.CloseTime = source.CloseTime
		c.Volume += source.Volume
		c.Turnover = c.Turnover.Add(source.Turnover)
	}
	return out
}

func maskAccount(v string) string {
	if len(v) <= 4 {
		return "••••"
	}
	return "••••" + v[len(v)-4:]
}
func str(m map[string]any, key string) string { return strings.TrimSpace(fmt.Sprint(m[key])) }
func first(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := str(m, k); v != "" && v != "<nil>" {
			return v
		}
	}
	return ""
}
func decAny(m map[string]any, keys ...string) decimal.Decimal {
	return parseDecimal(first(m, keys...)).Abs()
}
func decAnySigned(m map[string]any, keys ...string) decimal.Decimal {
	return parseDecimal(first(m, keys...))
}
func parseDecimal(v string) decimal.Decimal {
	d, _ := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(v), ",", ""))
	return d
}
func intAny(m map[string]any, keys ...string) int64 {
	v, _ := strconv.ParseInt(strings.ReplaceAll(first(m, keys...), ",", ""), 10, 64)
	if v < 0 {
		return -v
	}
	return v
}
