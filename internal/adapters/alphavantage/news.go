package alphavantage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/informationhttp"
	"github.com/77romin/minstock-tui/internal/domain"
)

func (*Client) InformationSource() string { return "Alpha Vantage" }
func (*Client) SupportsInformation(symbol domain.Symbol) bool {
	return symbol.Currency == domain.USD || symbol.Market == domain.MarketUS
}

func (c *Client) Information(ctx context.Context, symbol domain.Symbol) ([]domain.InformationItem, error) {
	ticker := strings.ToUpper(strings.TrimSpace(symbol.Ticker))
	if ticker == "" {
		ticker = strings.ToUpper(strings.TrimSpace(symbol.Code))
	}
	if ticker == "" {
		return nil, fmt.Errorf("미국 뉴스 조회에 티커가 필요합니다")
	}
	u, _ := url.Parse(c.baseURL)
	query := u.Query()
	query.Set("function", "NEWS_SENTIMENT")
	query.Set("tickers", ticker)
	query.Set("sort", "LATEST")
	query.Set("limit", "50")
	query.Set("apikey", c.apiKey)
	u.RawQuery = query.Encode()
	payload, err := informationhttp.Get(ctx, informationhttp.Client(), u.String(), nil, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("Alpha Vantage 뉴스: %w", err)
	}
	var response struct {
		Information string `json:"Information"`
		Note        string `json:"Note"`
		Error       string `json:"Error Message"`
		Feed        *[]struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Source  string `json:"source"`
			Time    string `json:"time_published"`
			Tickers []struct {
				Ticker    string `json:"ticker"`
				Relevance string `json:"relevance_score"`
			} `json:"ticker_sentiment"`
		} `json:"feed"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("Alpha Vantage 뉴스 응답 형식 오류")
	}
	if message := firstNonEmpty(response.Information, response.Note, response.Error); message != "" {
		lower := strings.ToLower(message)
		if strings.Contains(lower, "rate limit") || strings.Contains(lower, "25 requests") {
			return nil, fmt.Errorf("Alpha Vantage 일일 호출 한도를 초과했습니다")
		}
		if strings.Contains(lower, "premium") {
			return nil, fmt.Errorf("Alpha Vantage 뉴스가 현재 구독 범위에서 지원되지 않습니다")
		}
		return nil, fmt.Errorf("Alpha Vantage 뉴스 API 오류 · 키와 호출 한도를 확인하세요")
	}
	if response.Feed == nil {
		return nil, fmt.Errorf("Alpha Vantage 뉴스 feed 누락")
	}
	var result []domain.InformationItem
	for _, item := range *response.Feed {
		relevance := ""
		for _, mention := range item.Tickers {
			if strings.EqualFold(mention.Ticker, ticker) {
				relevance = mention.Relevance
				break
			}
		}
		if relevance == "" {
			continue
		}
		published, _ := time.Parse("20060102T150405", item.Time)
		result = append(result, domain.InformationItem{Kind: domain.InformationNews, Title: item.Title, URL: item.URL, Source: item.Source, PublishedAt: published, Relevance: relevance})
	}
	return domain.NormalizeInformation(result), nil
}
