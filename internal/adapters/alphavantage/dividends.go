package alphavantage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Alpha Vantage API key is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://www.alphavantage.co/query"
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid Alpha Vantage URL: %w", err)
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: strings.TrimSpace(apiKey), http: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (c *Client) DividendSource() string { return "Alpha Vantage" }

func (c *Client) Dividends(ctx context.Context, symbol string) ([]domain.DividendEvent, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	query := u.Query()
	query.Set("function", "DIVIDENDS")
	query.Set("symbol", strings.ToUpper(strings.TrimSpace(symbol)))
	query.Set("apikey", c.apiKey)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Alpha Vantage dividends HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Symbol      string `json:"symbol"`
		Information string `json:"Information"`
		Note        string `json:"Note"`
		Error       string `json:"Error Message"`
		Data        []struct {
			ExDate          string `json:"ex_dividend_date"`
			DeclarationDate string `json:"declaration_date"`
			RecordDate      string `json:"record_date"`
			PaymentDate     string `json:"payment_date"`
			Amount          any    `json:"amount"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if message := firstNonEmpty(payload.Error, payload.Note, payload.Information); message != "" {
		return nil, errors.New(message)
	}
	result := make([]domain.DividendEvent, 0, len(payload.Data))
	for _, item := range payload.Data {
		amount, err := decimal.NewFromString(strings.TrimSpace(fmt.Sprint(item.Amount)))
		if err != nil || !amount.IsPositive() {
			continue
		}
		result = append(result, domain.DividendEvent{
			Symbol: strings.ToUpper(strings.TrimSpace(symbol)), ExDate: parseDate(item.ExDate),
			DeclarationDate: parseDate(item.DeclarationDate), RecordDate: parseDate(item.RecordDate),
			PaymentDate: parseDate(item.PaymentDate), Amount: amount, Currency: domain.USD,
			Provider: c.DividendSource(), Freshness: domain.FreshLive,
		})
	}
	return result, nil
}

func parseDate(value string) time.Time {
	date, _ := time.Parse("2006-01-02", strings.TrimSpace(value))
	return date
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
