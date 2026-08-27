package kiwoom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	accountDomestic = "kiwoom-domestic"
	accountUS       = "kiwoom-us"
)

// This allowlist is an intentional safety boundary. The v0.1 program may call
// only documented read APIs; order, amendment, and cancellation IDs are absent.
var readOnlyAPIs = map[string]string{
	"kt00018":  "/api/dostk/acnt",
	"ka10001":  "/api/dostk/stkinfo",
	"ka10099":  "/api/dostk/stkinfo",
	"ka01300":  "/api/dostk/watchlist",
	"ka01301":  "/api/dostk/watchlist",
	"usa20200": "/api/us/watchlist",
	"usa20201": "/api/us/watchlist",
	"ka10079":  "/api/dostk/chart",
	"ka10080":  "/api/dostk/chart",
	"ka10081":  "/api/dostk/chart",
	"ka10082":  "/api/dostk/chart",
	"ka10083":  "/api/dostk/chart",
	"ka10094":  "/api/dostk/chart",
	"usa20100": "/api/us/mrkcond",
	"usa10099": "/api/us/stkinfo",
	"ust21070": "/api/us/acnt",
	"ust21110": "/api/us/acnt",
	"usa06010": "/api/us/chart",
	"usa06011": "/api/us/chart",
	"usa06012": "/api/us/chart",
	"usa06013": "/api/us/chart",
	"usa06014": "/api/us/chart",
	"usa06015": "/api/us/chart",
}

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
	allowedPath, allowed := readOnlyAPIs[apiID]
	if !allowed || allowedPath != path {
		return "", "", fmt.Errorf("kiwoom API %s %s is not in the read-only allowlist", apiID, path)
	}
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
	return []domain.Account{
		{ID: accountDomestic, Name: "키움 국내주식", Broker: c.ID(), Currency: domain.KRW, Type: "domestic"},
		{ID: accountUS, Name: "키움 미국주식", Broker: c.ID(), Currency: domain.USD, Type: "us"},
	}, nil
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
	if accountID == accountUS {
		return c.usBalance(ctx, accountID)
	}
	out, err := c.fetchBalance(ctx)
	if err != nil {
		return domain.Balance{}, err
	}
	return domain.Balance{AccountID: accountID, Broker: c.ID(), Currency: domain.KRW, PurchaseTotal: num(out.Purchase), ValueTotal: num(out.Value), ProfitLoss: signed(out.Profit), ProfitRate: signed(out.Rate), Cash: num(out.Assets).Sub(num(out.Value)), AsOf: time.Now()}, nil
}

func (c *Client) Positions(ctx context.Context, accountID string) ([]domain.Position, error) {
	if accountID == accountUS {
		return c.usPositions(ctx, accountID)
	}
	out, err := c.fetchBalance(ctx)
	if err != nil {
		return nil, err
	}
	positions := make([]domain.Position, 0, len(out.Positions))
	for _, p := range out.Positions {
		code := strings.TrimPrefix(strings.TrimSpace(p.Code), "A")
		positions = append(positions, domain.Position{AccountID: accountID, Broker: c.ID(), Symbol: domain.Symbol{Code: code, Ticker: code, Name: p.Name, Market: domain.MarketKRX, Currency: domain.KRW}, Quantity: num(p.Quantity), Tradable: num(p.Tradable), AveragePrice: num(p.Average), CurrentPrice: num(p.Price), PurchaseValue: num(p.Purchase), MarketValue: num(p.Value), ProfitLoss: signed(p.Profit), ProfitRate: signed(p.Rate), AsOf: time.Now()})
	}
	return positions, nil
}

type usBalanceResponse struct {
	Currency    string `json:"crnc_code"`
	Value       string `json:"tot_evlt_amt"`
	Purchase    string `json:"tot_prch_amt"`
	Profit      string `json:"tot_pl_amt"`
	Rate        string `json:"tot_pl_rt"`
	ValueKRW    string `json:"tot_evlt_amt_krw"`
	PurchaseKRW string `json:"tot_prch_amt_krw"`
	ProfitKRW   string `json:"tot_pl_amt_krw"`
	Positions   []struct {
		Exchange     string `json:"stex_nm"`
		Currency     string `json:"crnc_code"`
		Code         string `json:"stk_cd"`
		Name         string `json:"frgn_stk_nm"`
		Quantity     string `json:"poss_qty"`
		Tradable     string `json:"sell_alowq"`
		Average      string `json:"frgn_stk_book_uv"`
		Price        string `json:"now_pric"`
		Value        string `json:"evlt_amt"`
		Profit       string `json:"pl_amt"`
		Rate         string `json:"pl_rt"`
		Purchase     string `json:"frgn_stk_book_amt"`
		ExchangeRate string `json:"exch_rate"`
	} `json:"result_list"`
}

func (c *Client) fetchUSBalance(ctx context.Context) (usBalanceResponse, error) {
	var out usBalanceResponse
	err := c.call(ctx, "ust21070", "/api/us/acnt", map[string]string{"stex_tp": "", "stk_cd": ""}, &out)
	return out, err
}

func (c *Client) usBalance(ctx context.Context, accountID string) (domain.Balance, error) {
	out, err := c.fetchUSBalance(ctx)
	if err != nil {
		return domain.Balance{}, err
	}
	cash := decimal.Zero
	var deposit struct {
		Rows []struct {
			Currency string `json:"crnc_code"`
			Cash     string `json:"fc_entra"`
		} `json:"result_list"`
	}
	if err := c.call(ctx, "ust21110", "/api/us/acnt", map[string]string{}, &deposit); err == nil {
		for _, row := range deposit.Rows {
			if strings.EqualFold(row.Currency, "USD") {
				cash = num(row.Cash)
				break
			}
		}
	}
	return domain.Balance{AccountID: accountID, Broker: c.ID(), Currency: domain.USD, Cash: cash, PurchaseTotal: num(out.Purchase), ValueTotal: num(out.Value), ProfitLoss: signed(out.Profit), ProfitRate: signed(out.Rate), AsOf: time.Now()}, nil
}

func (c *Client) usPositions(ctx context.Context, accountID string) ([]domain.Position, error) {
	out, err := c.fetchUSBalance(ctx)
	if err != nil {
		return nil, err
	}
	positions := make([]domain.Position, 0, len(out.Positions))
	for _, p := range out.Positions {
		currency := domain.USD
		if p.Currency != "" && !strings.EqualFold(p.Currency, "USD") {
			continue // v0.1 overseas portfolio scope is US stocks only.
		}
		code := strings.TrimSpace(p.Code)
		symbol := domain.Symbol{Code: code, Ticker: code, Name: strings.TrimSpace(p.Name), Market: domain.MarketUS, Currency: currency, Exchange: normalizeUSExchange(p.Exchange)}
		positions = append(positions, domain.Position{AccountID: accountID, Broker: c.ID(), Symbol: symbol, Quantity: num(p.Quantity), Tradable: num(p.Tradable), AveragePrice: num(p.Average), CurrentPrice: num(p.Price), PurchaseValue: num(p.Purchase), MarketValue: num(p.Value), ProfitLoss: signed(p.Profit), ProfitRate: signed(p.Rate), AsOf: time.Now()})
	}
	return positions, nil
}

func (c *Client) Quote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	if symbol.Market == domain.MarketUS || symbol.Currency == domain.USD {
		return c.usQuote(ctx, symbol)
	}
	var out struct {
		Code         string `json:"stk_cd"`
		Name         string `json:"stk_nm"`
		Price        string `json:"cur_prc"`
		Open         string `json:"open_pric"`
		High         string `json:"high_pric"`
		Low          string `json:"low_pric"`
		Change       string `json:"pred_pre"`
		Rate         string `json:"flu_rt"`
		Volume       string `json:"trde_qty"`
		MarketCap    string `json:"mac"`
		MarketCapAlt string `json:"mkt_cap"`
		EPS          string `json:"eps"`
		PER          string `json:"per"`
	}
	if err := c.call(ctx, "ka10001", "/api/dostk/stkinfo", map[string]string{"stk_cd": symbol.Code}, &out); err != nil {
		return domain.Quote{}, err
	}
	if symbol.Name == "" {
		symbol.Name = out.Name
	}
	now := time.Now()
	marketCap := out.MarketCap
	if marketCap == "" {
		marketCap = out.MarketCapAlt
	}
	return domain.Quote{Symbol: symbol, Price: num(out.Price), Open: num(out.Open), High: num(out.High), Low: num(out.Low), Change: signed(out.Change), ChangeRate: signed(out.Rate), Volume: intNum(out.Volume), MarketCap: num(marketCap), EPS: signed(out.EPS), PER: signed(out.PER), MarketTime: now, ReceivedAt: now, Provider: c.ID(), Freshness: domain.FreshLive}, nil
}

func (c *Client) usQuote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	type response struct {
		Exchange     string `json:"stex_tp"`
		Code         string `json:"stk_cd"`
		Name         string `json:"stk_nm"`
		EnglishName  string `json:"stk_enm"`
		Price        string `json:"cur_prc"`
		Change       string `json:"pred_pre"`
		Rate         string `json:"flu_rt"`
		Volume       string `json:"acc_trde_qty"`
		Previous     string `json:"base_close_pric"`
		Open         string `json:"open_pric"`
		High         string `json:"high_pric"`
		Low          string `json:"low_pric"`
		PreOpen      string `json:"pre_open_pric"`
		PreHigh      string `json:"pre_high_pric"`
		PreLow       string `json:"pre_low_pric"`
		MarketCap    string `json:"mac"`
		MarketCapAlt string `json:"mkt_cap"`
		EPS          string `json:"eps"`
		PER          string `json:"per"`
	}
	var errs []error
	for _, exchange := range usExchangeCandidates(symbol.Exchange) {
		var out response
		if err := c.call(ctx, "usa20100", "/api/us/mrkcond", map[string]string{"stex_tp": exchange, "stk_cd": symbol.Code}, &out); err != nil {
			errs = append(errs, err)
			continue
		}
		if !num(out.Price).IsPositive() {
			errs = append(errs, fmt.Errorf("%s returned no price for %s", exchange, symbol.Code))
			continue
		}
		if symbol.Name == "" {
			symbol.Name = out.Name
			if symbol.Name == "" {
				symbol.Name = out.EnglishName
			}
		}
		symbol.Market, symbol.Currency, symbol.Exchange = domain.MarketUS, domain.USD, exchange
		open, high, low := out.Open, out.High, out.Low
		if open == "" {
			open = out.PreOpen
		}
		if high == "" {
			high = out.PreHigh
		}
		if low == "" {
			low = out.PreLow
		}
		now := time.Now()
		marketCap := out.MarketCap
		if marketCap == "" {
			marketCap = out.MarketCapAlt
		}
		return domain.Quote{Symbol: symbol, Price: num(out.Price), Previous: num(out.Previous), Open: num(open), High: num(high), Low: num(low), Change: signed(out.Change), ChangeRate: signed(out.Rate), Volume: intNum(out.Volume), MarketCap: num(marketCap), EPS: signed(out.EPS), PER: signed(out.PER), MarketTime: now, ReceivedAt: now, Provider: c.ID(), Freshness: domain.FreshLive, SourceLabel: exchange}, nil
	}
	return domain.Quote{}, errors.Join(errs...)
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
				result = append(result, domain.Symbol{Code: s.Code, Ticker: s.Code, Name: s.Name, Market: m.market, Currency: domain.KRW})
			}
			if !strings.EqualFold(continuation, "Y") || nextKey == "" {
				break
			}
		}
	}
	// Kiwoom exposes the US master through usa10099. It is intentionally
	// fetched only from an explicit sync (Service.Sync), never on startup.
	for _, exchange := range []string{"ND", "NY", "NA"} {
		continuation, nextKey := "", ""
		for {
			var raw map[string]json.RawMessage
			var err error
			continuation, nextKey, err = c.callPage(ctx, "usa10099", "/api/us/stkinfo", map[string]string{"stex_tp": exchange}, &raw, continuation, nextKey)
			if err != nil {
				return nil, err
			}
			for _, item := range parseUSInstrumentItems(raw) {
				code := strings.TrimSpace(firstString(item, "stk_cd", "code", "symbol", "ticker"))
				name := strings.TrimSpace(firstString(item, "stk_nm", "name", "display_name"))
				if code == "" {
					continue
				}
				result = append(result, domain.Symbol{Code: code, Ticker: code, Name: name, Market: domain.MarketUS, Currency: domain.USD, Exchange: exchange})
			}
			if !strings.EqualFold(continuation, "Y") || nextKey == "" {
				break
			}
		}
	}
	return result, nil
}

// parseUSInstrumentItems accepts the current API envelope (us_stklist) and
// a few historical aliases so a server-side field rename does not silently
// remove every US result from local search.
func parseUSInstrumentItems(raw map[string]json.RawMessage) []map[string]any {
	for _, key := range []string{"us_stklist", "result_list", "list", "items", "stk_list"} {
		if b, ok := raw[key]; ok {
			var items []map[string]any
			if json.Unmarshal(b, &items) == nil {
				return items
			}
		}
	}
	// Be tolerant of an envelope rename: locate the first array containing
	// objects with a stock-code field anywhere in the response tree.
	for _, value := range raw {
		var node any
		if json.Unmarshal(value, &node) == nil {
			if items := findUSInstrumentArray(node); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func findUSInstrumentArray(node any) []map[string]any {
	switch value := node.(type) {
	case []any:
		items := make([]map[string]any, 0, len(value))
		for _, child := range value {
			item, ok := child.(map[string]any)
			if !ok {
				continue
			}
			if firstString(item, "stk_cd", "code", "symbol", "ticker") != "" {
				items = append(items, item)
			}
		}
		if len(items) > 0 {
			return items
		}
		for _, child := range value {
			if items := findUSInstrumentArray(child); len(items) > 0 {
				return items
			}
		}
	case map[string]any:
		for _, child := range value {
			if items := findUSInstrumentArray(child); len(items) > 0 {
				return items
			}
		}
	}
	return nil
}

func firstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			switch v := value.(type) {
			case string:
				return v
			case json.Number:
				return v.String()
			case float64:
				return strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
	}
	return ""
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
	// Kiwoom exposes domestic and US watchlists through separate TRs. Prefix
	// US group IDs so they cannot collide with domestic group numbers in SQLite.
	var usRaw json.RawMessage
	if err := c.call(ctx, "usa20200", "/api/us/watchlist", map[string]string{}, &usRaw); err == nil {
		for _, g := range parseUSWatchlistGroups(usRaw) {
			id := "US:" + g.Code
			groups = append(groups, domain.WatchlistGroup{ID: id, ExternalID: id, Name: g.Name, Provider: c.ID()})
		}
	}
	return groups, nil
}

func (c *Client) WatchlistItems(ctx context.Context, groupID string) ([]domain.WatchlistItem, error) {
	if strings.HasPrefix(groupID, "US:") {
		var raw json.RawMessage
		groupCode := strings.TrimPrefix(groupID, "US:")
		// Kiwoom deployments have used both grp_no and arn_grp_id for this
		// account-scoped request; sending both keeps the client compatible.
		if err := c.call(ctx, "usa20201", "/api/us/watchlist", map[string]string{"grp_no": groupCode, "gcod": groupCode, "arn_grp_id": groupCode}, &raw); err != nil {
			return nil, err
		}
		items := make([]domain.WatchlistItem, 0)
		for _, item := range parseUSWatchlistItems(raw) {
			items = append(items, domain.WatchlistItem{GroupID: groupID, Provider: c.ID(), Symbol: domain.Symbol{Code: item.Code, Ticker: item.Code, Name: item.Name, Market: domain.MarketUS, Currency: domain.USD, Exchange: item.Exchange}})
		}
		return items, nil
	}
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
		items = append(items, domain.WatchlistItem{GroupID: groupID, Provider: c.ID(), Symbol: domain.Symbol{Code: i.Code, Ticker: i.Code, Market: domain.MarketKRX, Currency: domain.KRW}})
	}
	return items, nil
}

type usWatchlistEntry struct{ Code, Name, Exchange string }

func parseUSWatchlistGroups(raw json.RawMessage) []usWatchlistEntry {
	return findUSWatchlistEntries(raw, []string{"grp_no", "grp_id", "gcod", "group_no", "group_id", "id", "seq"}, []string{"grp_nm", "group_nm", "name", "group_name", "groupname"})
}

func parseUSWatchlistItems(raw json.RawMessage) []usWatchlistEntry {
	return findUSWatchlistEntries(raw, []string{"stk_cd", "cod2", "ticker", "symbol", "code"}, []string{"stk_nm", "name", "display_name"})
}

func findUSWatchlistEntries(raw json.RawMessage, codeKeys, nameKeys []string) []usWatchlistEntry {
	var walk func(any) []usWatchlistEntry
	walk = func(v any) []usWatchlistEntry {
		switch x := v.(type) {
		case []any:
			out := make([]usWatchlistEntry, 0)
			for _, child := range x {
				out = append(out, walk(child)...)
			}
			return out
		case map[string]any:
			code, name := firstString(x, codeKeys...), firstString(x, nameKeys...)
			if code != "" {
				return []usWatchlistEntry{{Code: code, Name: name, Exchange: firstString(x, "stex_tp", "exchange", "exch")}}
			}
			out := make([]usWatchlistEntry, 0)
			for _, child := range x {
				out = append(out, walk(child)...)
			}
			return out
		}
		return nil
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return walk(decoded)
}

func (c *Client) Candles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	if q.Symbol.Market == domain.MarketUS || q.Symbol.Currency == domain.USD {
		return c.usCandles(ctx, q)
	}
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

func (c *Client) usCandles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	apiID := map[domain.CandleInterval]string{
		domain.IntervalTick: "usa06010", domain.Interval1Min: "usa06011",
		domain.Interval5Min: "usa06011", domain.Interval15Min: "usa06011",
		domain.Interval60Min: "usa06011", domain.IntervalDay: "usa06012",
		domain.IntervalWeek: "usa06013", domain.IntervalMonth: "usa06014",
		domain.IntervalYear: "usa06015",
	}[q.Interval]
	if apiID == "" {
		return nil, fmt.Errorf("unsupported US candle interval %s", q.Interval)
	}
	var errs []error
	for _, exchange := range usExchangeCandidates(q.Symbol.Exchange) {
		body := map[string]string{
			"stex_tp": exchange, "stk_cd": q.Symbol.Code,
			"upd_stkpc_tp": map[bool]string{true: "1", false: "0"}[q.Adjusted],
			"exrt_appl_tp": "0",
		}
		switch q.Interval {
		case domain.IntervalTick:
			body["tic_scope"] = "1"
		case domain.Interval1Min, domain.Interval5Min, domain.Interval15Min, domain.Interval60Min:
			body["strt_dt"] = q.From.Format("20060102")
			body["tic_scope"] = strings.TrimSuffix(string(q.Interval), "m")
		default:
			body["strt_dt"] = q.From.Format("20060102")
		}
		var out struct {
			Rows []struct {
				Close             string `json:"cur_prc"`
				TradeVolume       string `json:"trde_qty"`
				AccumulatedVolume string `json:"acc_trde_qty"`
				Turnover          string `json:"acc_trde_prica"`
				Open              string `json:"open_pric"`
				High              string `json:"high_pric"`
				Low               string `json:"low_pric"`
				Time              string `json:"cntr_tm"`
				Date              string `json:"dt"`
			} `json:"result_list"`
		}
		if err := c.call(ctx, apiID, "/api/us/chart", body, &out); err != nil {
			errs = append(errs, err)
			continue
		}
		if len(out.Rows) == 0 {
			errs = append(errs, fmt.Errorf("%s returned no %s candles for %s", exchange, q.Interval, q.Symbol.Code))
			continue
		}
		limit := q.Limit
		if limit <= 0 || limit > len(out.Rows) {
			limit = len(out.Rows)
		}
		loc := time.FixedZone("KST", 9*60*60)
		candles := make([]domain.Candle, 0, limit)
		symbol := q.Symbol
		symbol.Market, symbol.Currency, symbol.Exchange = domain.MarketUS, domain.USD, exchange
		for _, row := range out.Rows[:limit] {
			stamp, layout := row.Date, "20060102"
			if row.Time != "" {
				stamp, layout = row.Time, "20060102150405"
			}
			openTime, err := time.ParseInLocation(layout, stamp, loc)
			if err != nil {
				continue
			}
			volume := row.TradeVolume
			if volume == "" {
				volume = row.AccumulatedVolume
			}
			candles = append(candles, domain.Candle{Symbol: symbol, Interval: q.Interval, OpenTime: openTime, CloseTime: closeTime(openTime, q.Interval), Open: num(row.Open), High: num(row.High), Low: num(row.Low), Close: num(row.Close), Volume: intNum(volume), Turnover: num(row.Turnover), Adjusted: q.Adjusted, Complete: true, Provider: c.ID()})
		}
		sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime.Before(candles[j].OpenTime) })
		return candles, nil
	}
	return nil, errors.Join(errs...)
}

func normalizeUSExchange(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	switch {
	case v == "ND" || strings.Contains(v, "NASDAQ") || strings.Contains(v, "나스닥"):
		return "ND"
	case v == "NY" || strings.Contains(v, "NYSE") || strings.Contains(v, "뉴욕"):
		return "NY"
	case v == "NA" || strings.Contains(v, "AMEX") || strings.Contains(v, "아멕스"):
		return "NA"
	default:
		return ""
	}
}

func usExchangeCandidates(preferred string) []string {
	preferred = normalizeUSExchange(preferred)
	all := []string{"ND", "NY", "NA"}
	if preferred == "" {
		return all
	}
	result := []string{preferred}
	for _, exchange := range all {
		if exchange != preferred {
			result = append(result, exchange)
		}
	}
	return result
}

func closeTime(t time.Time, i domain.CandleInterval) time.Time {
	switch i {
	case domain.IntervalTick:
		return t.Add(time.Second)
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
