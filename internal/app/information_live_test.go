package app_test

import (
	"context"
	"github.com/77romin/minstock-tui/internal/adapters/alphavantage"
	"github.com/77romin/minstock-tui/internal/adapters/dart"
	"github.com/77romin/minstock-tui/internal/adapters/naver"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/77romin/minstock-tui/internal/security"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in: consumes API quota, reads credentials, and never writes keys.
func TestInformationLiveSmoke(t *testing.T) {
	if os.Getenv("MINSTOCK_INFORMATION_SMOKE") != "1" {
		t.Skip("opt-in live information test")
	}
	repo, err := db.Open(filepath.Join(t.TempDir(), "information.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	var providers []ports.InformationProvider
	if creds, err := security.Load("naver"); err == nil {
		p, err := naver.New(config.NaverNewsURL, creds)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("NAVER key unavailable")
	}
	if key, _, err := security.LoadAPIKey("dart", "DART_API_KEY"); err == nil {
		p, err := dart.New("https://opendart.fss.or.kr/api", key, repo)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("DART key unavailable")
	}
	if key, _, err := security.LoadAPIKey("alphavantage", "ALPHAVANTAGE_API_KEY"); err == nil {
		p, err := alphavantage.New("https://www.alphavantage.co/query", key)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("Alpha Vantage key unavailable")
	}
	if len(providers) == 0 {
		t.Skip("no configured providers")
	}
	s := app.New(repo, nil, nil, nil, nil)
	s.SetInformationProviders(providers)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	for _, symbol := range []domain.Symbol{{Code: "005930", Name: "삼성전자", Currency: domain.KRW}, {Code: "AAPL", Ticker: "AAPL", Currency: domain.USD}} {
		report := s.Information(ctx, symbol)
		for _, source := range report.Sources {
			t.Logf("%s %s: items=%d warning=%s", symbol.Code, source.Source, len(report.Items), source.Warning)
			if source.Warning != "" {
				t.Errorf("%s live request did not succeed", source.Source)
			}
		}
	}
}
