package naver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/informationhttp"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
)

type Client struct {
	baseURL     string
	credentials security.Credentials
	http        *http.Client
}

func New(baseURL string, credentials security.Credentials) (*Client, error) {
	if strings.TrimSpace(credentials.AppKey) == "" || strings.TrimSpace(credentials.Secret) == "" {
		return nil, fmt.Errorf("NAVER API HUB Client ID/Secret 필요")
	}
	endpoint, err := informationhttp.Endpoint(baseURL, "naverapihub.apigw.ntruss.com")
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: endpoint, credentials: credentials, http: informationhttp.Client()}, nil
}
func (*Client) InformationSource() string { return "NAVER" }
func (*Client) SupportsInformation(symbol domain.Symbol) bool {
	return symbol.Currency != domain.USD && symbol.Market != domain.MarketUS
}

func (c *Client) Information(ctx context.Context, symbol domain.Symbol) ([]domain.InformationItem, error) {
	name := domain.InformationText(symbol.Name)
	if name == "" || name == symbol.Code {
		return nil, fmt.Errorf("NAVER 뉴스 검색에 종목명이 필요합니다 · 종목 동기화를 확인하세요")
	}
	u, _ := url.Parse(c.baseURL)
	query := u.Query()
	query.Set("query", name)
	query.Set("display", "50")
	query.Set("sort", "date")
	query.Set("format", "json")
	u.RawQuery = query.Encode()
	payload, err := informationhttp.Get(ctx, c.http, u.String(), map[string]string{"X-NCP-APIGW-API-KEY-ID": c.credentials.AppKey, "X-NCP-APIGW-API-KEY": c.credentials.Secret}, 4<<20)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "HTTP 403") {
			return nil, fmt.Errorf("NAVER API HUB 뉴스: %w · API HUB 키와 뉴스 API 권한을 확인하세요 (minstock setup naver)", err)
		}
		return nil, fmt.Errorf("NAVER API HUB 뉴스: %w", err)
	}
	var response struct {
		Items *[]struct {
			Title       string `json:"title"`
			Original    string `json:"originallink"`
			Link        string `json:"link"`
			Description string `json:"description"`
			Date        string `json:"pubDate"`
		} `json:"items"`
	}
	if err := json.Unmarshal(payload, &response); err != nil || response.Items == nil {
		return nil, fmt.Errorf("NAVER 뉴스 응답 형식 오류")
	}
	var result []domain.InformationItem
	for _, item := range *response.Items {
		title := domain.InformationText(item.Title)
		if !strings.Contains(strings.ToLower(title+" "+domain.InformationText(item.Description)), strings.ToLower(name)) {
			continue
		}
		link := domain.InformationURL(item.Original)
		if link == "" {
			link = domain.InformationURL(item.Link)
		}
		parsed, _ := url.Parse(link)
		source := parsed.Hostname()
		published, err := time.Parse(time.RFC1123Z, item.Date)
		if err != nil {
			published, _ = time.Parse(time.RFC1123, item.Date)
		}
		result = append(result, domain.InformationItem{Kind: domain.InformationNews, Title: title, URL: link, Source: source, PublishedAt: published, Relevance: "종목명 검색"})
	}
	return domain.NormalizeInformation(result), nil
}
