package dart

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/informationhttp"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

type Client struct {
	baseURL, apiKey string
	http            *http.Client
	repo            ports.CacheRepository
	mu              sync.Mutex
	codes           map[string]string
	codesAt         time.Time
}

func New(baseURL, apiKey string, repo ports.CacheRepository) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("DART API 키 필요")
	}
	endpoint, err := informationhttp.Endpoint(baseURL, "opendart.fss.or.kr")
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: strings.TrimRight(endpoint, "/"), apiKey: apiKey, http: informationhttp.Client(), repo: repo}, nil
}
func (*Client) InformationSource() string { return "DART" }
func (*Client) SupportsInformation(symbol domain.Symbol) bool {
	return symbol.Currency != domain.USD && symbol.Market != domain.MarketUS
}
func (c *Client) get(ctx context.Context, path string, values url.Values, limit int64) ([]byte, error) {
	values.Set("crtfc_key", c.apiKey)
	return informationhttp.Get(ctx, c.http, c.baseURL+path+"?"+values.Encode(), nil, limit)
}

func (c *Client) corporation(ctx context.Context, code string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.codes == nil && c.repo != nil {
		if payload, stamp, err := c.repo.LoadCache(ctx, "dart-corporations:v1"); err == nil && json.Unmarshal(payload, &c.codes) == nil {
			c.codesAt = stamp
		}
	}
	if c.codes == nil || time.Since(c.codesAt) >= 24*time.Hour {
		payload, err := c.get(ctx, "/corpCode.xml", url.Values{}, 32<<20)
		if err != nil {
			return "", fmt.Errorf("DART 회사 매핑: %w", err)
		}
		reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
		if err != nil {
			var failure struct {
				Status string `xml:"status"`
			}
			_ = xml.Unmarshal(payload, &failure)
			return "", fmt.Errorf("DART 회사 매핑 응답 오류 (상태 %s)", failure.Status)
		}
		codes := map[string]string{}
		found := false
		for _, file := range reader.File {
			if !strings.EqualFold(file.Name, "CORPCODE.xml") {
				continue
			}
			found = true
			if file.UncompressedSize64 > 128<<20 {
				return "", fmt.Errorf("DART 회사 매핑 크기 초과")
			}
			data, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("DART 매핑 ZIP 읽기 실패")
			}
			decoder := xml.NewDecoder(io.LimitReader(data, 128<<20))
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					data.Close()
					return "", fmt.Errorf("DART 회사 매핑 XML 오류")
				}
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == "list" {
					var row struct {
						Corporation string `xml:"corp_code"`
						Stock       string `xml:"stock_code"`
					}
					if err := decoder.DecodeElement(&row, &start); err != nil {
						data.Close()
						return "", fmt.Errorf("DART 회사 매핑 XML 행 오류")
					}
					if len(strings.TrimSpace(row.Stock)) == 6 && len(row.Corporation) == 8 {
						codes[strings.TrimSpace(row.Stock)] = row.Corporation
					}
				}
			}
			data.Close()
		}
		if !found || len(codes) == 0 {
			return "", fmt.Errorf("DART 회사 매핑에 상장 종목 없음")
		}
		c.codes, c.codesAt = codes, time.Now()
		if c.repo != nil {
			encoded, _ := json.Marshal(codes)
			_ = c.repo.SaveCache(ctx, "dart-corporations:v1", encoded)
		}
	}
	corporation := c.codes[code]
	if corporation == "" {
		return "", fmt.Errorf("DART 공시 대상 회사가 아닙니다 · ETF 등은 미지원")
	}
	return corporation, nil
}

func (c *Client) Information(ctx context.Context, symbol domain.Symbol) ([]domain.InformationItem, error) {
	corporation, err := c.corporation(ctx, symbol.Code)
	if err != nil {
		return nil, err
	}
	loc := time.FixedZone("KST", 9*60*60)
	now := time.Now().In(loc)
	values := url.Values{"corp_code": {corporation}, "bgn_de": {now.AddDate(0, 0, -90).Format("20060102")}, "end_de": {now.Format("20060102")}, "page_count": {"50"}, "page_no": {"1"}, "sort": {"date"}, "sort_mth": {"desc"}}
	payload, err := c.get(ctx, "/list.json", values, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("DART 공시: %w", err)
	}
	var response struct {
		Status string `json:"status"`
		List   []struct {
			Corporation string `json:"corp_code"`
			Stock       string `json:"stock_code"`
			Number      string `json:"rcept_no"`
			Title       string `json:"report_nm"`
			Date        string `json:"rcept_dt"`
		} `json:"list"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return nil, fmt.Errorf("DART 공시 응답 형식 오류")
	}
	if response.Status == "013" {
		return nil, nil
	}
	if response.Status != "000" {
		return nil, fmt.Errorf("DART 공시 오류 (상태 %s) · API 키와 호출 한도를 확인하세요", response.Status)
	}
	var result []domain.InformationItem
	for _, row := range response.List {
		if row.Corporation != corporation || (row.Stock != "" && row.Stock != symbol.Code) || len(row.Number) != 14 {
			continue
		}
		published, _ := time.ParseInLocation("20060102", row.Date, loc)
		result = append(result, domain.InformationItem{Kind: domain.InformationDisclosure, Title: row.Title, URL: "https://dart.fss.or.kr/dsaf001/main.do?" + url.Values{"rcpNo": {row.Number}}.Encode(), Source: "DART", PublishedAt: published, DateOnly: true, Relevance: "종목코드 일치"})
	}
	return domain.NormalizeInformation(result), nil
}
