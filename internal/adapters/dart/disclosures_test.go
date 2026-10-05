package dart

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/77romin/minstock-tui/internal/domain"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memoryCache struct {
	mu      sync.Mutex
	payload []byte
}

func (m *memoryCache) SaveCache(_ context.Context, _ string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payload = payload
	return nil
}
func (m *memoryCache) LoadCache(context.Context, string) ([]byte, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.payload == nil {
		return nil, time.Time{}, fmt.Errorf("missing")
	}
	return m.payload, time.Now(), nil
}

func TestDisclosuresUseStockCorporationMappingAndPersistentCache(t *testing.T) {
	var compressed bytes.Buffer
	archive := zip.NewWriter(&compressed)
	file, _ := archive.Create("CORPCODE.xml")
	file.Write([]byte(`<result><list><corp_code>00126380</corp_code><stock_code>005930</stock_code></list></result>`))
	archive.Close()
	mapCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("crtfc_key") != "test-key" {
			t.Error("missing key")
		}
		if r.URL.Path == "/corpCode.xml" {
			mapCalls++
			w.Write(compressed.Bytes())
			return
		}
		if r.URL.Path != "/list.json" || r.URL.Query().Get("corp_code") != "00126380" || r.URL.Query().Get("page_count") != "50" {
			t.Errorf("incorrect query: %v", r.URL.Query())
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "000", "list": []map[string]string{
			{"corp_code": "00126380", "stock_code": "005930", "rcept_no": "20261005000001", "report_nm": "분기보고서", "rcept_dt": "20261005"},
			{"corp_code": "99999999", "stock_code": "000660", "rcept_no": "20261005000002", "report_nm": "다른 회사", "rcept_dt": "20261005"},
		}})
	}))
	defer server.Close()
	cache := &memoryCache{}
	for i := 0; i < 2; i++ {
		client, _ := New(server.URL, "test-key", cache)
		items, err := client.Information(t.Context(), domain.Symbol{Code: "005930"})
		if err != nil || len(items) != 1 || !items[0].DateOnly || items[0].URL != "https://dart.fss.or.kr/dsaf001/main.do?rcpNo=20261005000001" {
			t.Fatalf("items=%#v err=%v", items, err)
		}
	}
	if mapCalls != 1 {
		t.Fatalf("mapping re-downloaded %d times", mapCalls)
	}
}

func TestMappingRejectsXMLApiErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<result><status>010</status><message>invalid key</message></result>`))
	}))
	defer server.Close()
	client, _ := New(server.URL, "test-key", nil)
	if _, err := client.Information(t.Context(), domain.Symbol{Code: "005930"}); err == nil {
		t.Fatal("API XML error accepted as no disclosures")
	}
}
