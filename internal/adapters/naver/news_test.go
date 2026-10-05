package naver

import (
	"encoding/json"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewsContractAndCompanyFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/v1/news" || r.URL.Query().Get("query") != "삼성전자" || r.URL.Query().Get("sort") != "date" || r.URL.Query().Get("display") != "50" || r.URL.Query().Get("format") != "json" || r.Header.Get("X-NCP-APIGW-API-KEY-ID") != "id" || r.Header.Get("X-NCP-APIGW-API-KEY") != "secret" {
			t.Errorf("unexpected request parameters/headers")
		}
		if r.Header.Get("X-Naver-Client-Id") != "" || r.Header.Get("X-Naver-Client-Secret") != "" || r.URL.Query().Has("apikey") {
			t.Error("legacy authentication or URL credentials present")
		}
		json.NewEncoder(w).Encode(map[string]any{"items": []map[string]string{
			{"title": "<b>삼성전자</b> &amp; 실적", "originallink": "https://press.example/article?id=1&utm_campaign=test", "pubDate": "Mon, 05 Oct 2026 08:00:00 +0900"},
			{"title": "다른 회사", "description": "종목과 무관한 기사", "link": "https://press.example/other", "pubDate": "Mon, 05 Oct 2026 07:00:00 +0900"},
			{"title": "시장 전망", "description": "<b>삼성전자</b> 관련 내용", "originallink": "javascript:unsafe", "link": "https://news.naver.com/article/1", "pubDate": "Mon, 05 Oct 2026 06:00:00 +0900"},
		}})
	}))
	defer server.Close()
	client, err := New(server.URL+"/search/v1/news", security.Credentials{AppKey: "id", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.Information(t.Context(), domain.Symbol{Name: "삼성전자", Code: "005930", Currency: domain.KRW})
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].Title != "삼성전자 & 실적" || items[0].Source != "press.example" || items[0].PublishedAt.Hour() != 8 || items[0].URL != "https://press.example/article?id=1" {
		t.Fatalf("mapping: %#v", items[0])
	}
}

func TestNewsAPIHubEndpointAndAuthenticationErrors(t *testing.T) {
	creds := security.Credentials{AppKey: "id", Secret: "secret"}
	if _, err := New("https://naverapihub.apigw.ntruss.com/search/v1/news", creds); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"https://openapi.naver.com/v1/search/news.json",
		"http://naverapihub.apigw.ntruss.com/search/v1/news",
		"https://naverapihub.apigw.ntruss.com.evil.example/search/v1/news",
	} {
		if _, err := New(endpoint, creds); err == nil {
			t.Fatal("unsafe or legacy endpoint accepted")
		}
	}
	for _, status := range []int{401, 403, 429} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); w.Write([]byte("id secret")) }))
		client, _ := New(server.URL, creds)
		_, err := client.Information(t.Context(), domain.Symbol{Name: "삼성전자"})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unredacted API error: %v", err)
		}
		if status != 429 && !strings.Contains(err.Error(), "minstock setup naver") {
			t.Fatal("missing authentication guidance")
		}
		if status == 429 && !strings.Contains(err.Error(), "HTTP 429") {
			t.Fatal("quota status lost")
		}
	}
}

func TestNewsMalformedPayloadIsNotNoNews(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"errorCode":"bad"}`)) }))
	defer server.Close()
	client, _ := New(server.URL, security.Credentials{AppKey: "id", Secret: "secret"})
	if _, err := client.Information(t.Context(), domain.Symbol{Name: "삼성전자"}); err == nil {
		t.Fatal("error response accepted as empty news")
	}
	if _, err := New("http://untrusted.example/news", security.Credentials{AppKey: "id", Secret: "secret"}); err == nil {
		t.Fatal("untrusted API endpoint accepted")
	}
}
