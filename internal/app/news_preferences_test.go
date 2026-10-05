package app

import (
	"path/filepath"
	"testing"

	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
)

func TestNewsPreferencesPersistWithoutProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "news.db")
	repo, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := New(repo, nil, nil, nil, nil)
	if p, err := s.NewsPreferences(t.Context()); err != nil || p.DirectMention {
		t.Fatalf("default: %+v %v", p, err)
	}
	p := domain.NewsPreferences{DirectMention: true, MinRelevance: 0.6, HideTemplates: true, Keyword: "ETF", HiddenSources: []string{"CoinGecko", "coingecko"}}
	if err := s.SaveNewsPreferences(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	s = New(repo, nil, nil, nil, nil)
	p, err = s.NewsPreferences(t.Context())
	if err != nil || !p.DirectMention || !p.HideTemplates || p.MinRelevance != 0.6 || p.Keyword != "ETF" || len(p.HiddenSources) != 1 {
		t.Fatalf("restart: %+v %v", p, err)
	}
}
