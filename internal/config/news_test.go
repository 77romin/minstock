package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNaverAPIHubDefaultAndLegacyConfigUpgrade(t *testing.T) {
	if Default().News.NaverBaseURL != NaverNewsURL {
		t.Fatal("default is not NAVER API HUB")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	old := []byte("[news]\nnaver_base_url = \"https://openapi.naver.com/v1/search/news.json\"\n")
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.News.NaverBaseURL != NaverNewsURL {
		t.Fatalf("legacy migration: url=%s err=%v", cfg.News.NaverBaseURL, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(old) {
		t.Fatal("loading overwrote user configuration")
	}
}

func TestNaverAPIHubDoesNotOverwriteExplicitEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[news]\nnaver_base_url = \"http://localhost:12345/search/v1/news\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.News.NaverBaseURL != "http://localhost:12345/search/v1/news" {
		t.Fatal("custom endpoint overwritten")
	}
}
