package domain

import (
	"strings"
	"testing"
	"time"
)

func TestInformationNormalizationRemovesMarkupAndDeduplicates(t *testing.T) {
	now := time.Now()
	items := []InformationItem{
		{Kind: InformationNews, Title: "<b>삼성전자</b> &amp; 실적\x1b[31m", Source: "신문\n사", URL: "https://NEWS.example/a?utm_source=test&id=1#part", PublishedAt: now},
		{Kind: InformationNews, Title: "삼성전자 & 실적", URL: "https://news.example/other", PublishedAt: now.Add(-time.Minute)},
		{Kind: InformationNews, Title: "다른 제목", URL: "https://news.example/a?id=1", PublishedAt: now.Add(-2 * time.Minute)},
		{Kind: InformationNews, Title: "unsafe", URL: "javascript:alert(1)", PublishedAt: now},
		{Kind: InformationDisclosure, Title: "분기보고서", URL: "https://dart.fss.or.kr/a?rcpNo=1", PublishedAt: now},
		{Kind: InformationDisclosure, Title: "분기보고서", URL: "https://dart.fss.or.kr/a?rcpNo=2", PublishedAt: now},
	}
	result := NormalizeInformation(items)
	if len(result) != 3 {
		t.Fatalf("dedupe: %#v", result)
	}
	if result[0].Title != "삼성전자 & 실적" || result[0].URL != "https://news.example/a?id=1" || strings.Contains(result[0].Source, "\n") {
		t.Fatalf("sanitization: %#v", result[0])
	}
	for _, raw := range []string{"file:///etc/passwd", "https://user:secret@host/a", "https://host/a\x1b", "/relative", "ftp://host/a"} {
		if InformationURL(raw) != "" {
			t.Fatalf("unsafe URL accepted: %q", raw)
		}
	}
}
