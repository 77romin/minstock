package domain

import "testing"

func TestNewsQualityFilters(t *testing.T) {
	p := NewsPreferences{DirectMention: true}
	symbols := []Symbol{{Code: "QQQ", Ticker: "QQQ", Name: "QQQ"}}
	for _, title := range []string{"RSQQQ to KRW", "TQQQ price prediction", "SQQQ forecast"} {
		if p.Allows(InformationItem{Kind: InformationNews, Title: title}, symbols) {
			t.Fatalf("false QQQ match: %s", title)
		}
	}
	for _, title := range []string{"QQQ drops today", "Invesco (qqq): earnings", "QQQ ETF outlook"} {
		if !p.Allows(InformationItem{Kind: InformationNews, Title: title}, symbols) {
			t.Fatalf("missing QQQ match: %s", title)
		}
	}
	if !p.Allows(InformationItem{Kind: InformationDisclosure, Title: "공시"}, symbols) {
		t.Fatal("disclosure filtered by direct mention")
	}
	p = NewsPreferences{MinRelevance: 0.6, HiddenSources: []string{"CoinGecko"}}
	if p.Allows(InformationItem{Kind: InformationNews, Source: "coingecko", Relevance: "1.0"}, nil) {
		t.Fatal("hidden source visible")
	}
	if p.Allows(InformationItem{Kind: InformationNews, Source: "Yahoo", Relevance: "0.3"}, nil) {
		t.Fatal("low relevance visible")
	}
	if !p.Allows(InformationItem{Kind: InformationNews, Source: "NAVER", Relevance: ""}, nil) {
		t.Fatal("provider without relevance hidden")
	}
	p.Keyword = "배당"
	if !p.Allows(InformationItem{Kind: InformationDisclosure, Title: "현금 배당 결정"}, nil) || p.Allows(InformationItem{Kind: InformationDisclosure, Title: "실적 발표"}, nil) {
		t.Fatal("keyword filter failed")
	}
}

func TestDirectMentionKoreanParticlesAndShortTickerAmbiguity(t *testing.T) {
	p := NewsPreferences{DirectMention: true}
	kr := []Symbol{{Code: "005930", Name: "삼성전자"}}
	for _, title := range []string{"삼성전자는 실적을 발표했다", "삼성전자의 신규 투자", "삼성전자에서 공개한 기술", "삼성전자에는 기회", "삼성전자, 반도체 수주"} {
		if !p.Allows(InformationItem{Title: title}, kr) {
			t.Errorf("missed Korean mention: %s", title)
		}
	}
	for _, title := range []string{"삼성전자우 주가 상승", "가짜삼성전자 뉴스", "삼성전자관련주 상승"} {
		if p.Allows(InformationItem{Title: title}, kr) {
			t.Errorf("incorrect Korean mention: %s", title)
		}
	}
	for _, title := range []string{"A stock market update", "a new company", "Plan A: company outlook", "Visa faces a slowdown"} {
		if p.Allows(InformationItem{Title: title}, []Symbol{{Ticker: "A", Name: "A"}}) {
			t.Errorf("article mistaken for A: %s", title)
		}
	}
	for _, title := range []string{"$a earnings", "Agilent (A) earnings", "NYSE:A earnings"} {
		if !p.Allows(InformationItem{Title: title}, []Symbol{{Ticker: "A"}}) {
			t.Errorf("explicit A missed: %s", title)
		}
	}
	for _, title := range []string{"ON Semiconductor reports earnings", "Onsemi (on) reports earnings", "İ ON earnings"} {
		if !p.Allows(InformationItem{Title: title}, []Symbol{{Ticker: "ON"}}) {
			t.Errorf("explicit ON missed: %s", title)
		}
	}
	if p.Allows(InformationItem{Title: "Investors focus on earnings"}, []Symbol{{Ticker: "ON"}}) {
		t.Fatal("preposition mistaken for ON")
	}
	if !p.Allows(InformationItem{Title: "BRK.B earnings"}, []Symbol{{Ticker: "BRK.B"}}) {
		t.Fatal("class ticker lost")
	}
}

func TestTemplateFilterIsOptionalAndConservative(t *testing.T) {
	p := NewsPreferences{HideTemplates: true}
	for _, title := range []string{"RSQQQ to KRW: latest quote", "$QQQ to USD price", "ProShares stock price in South Korean Won", "QQQ Price Prediction 2026-2050", "QQQ price forecast for 2027", "QQQ 가격 예측 2027년"} {
		item := InformationItem{Kind: InformationNews, Title: title, Relevance: "1"}
		if p.ExclusionReason(item, nil) != NewsExcludeTemplate {
			t.Errorf("template not excluded: %s", title)
		}
		if !(NewsPreferences{}).Allows(item, nil) {
			t.Fatal("templates hidden by default")
		}
		item.Kind = InformationDisclosure
		if !p.Allows(item, nil) {
			t.Fatal("template rule applied to disclosure")
		}
	}
	for _, title := range []string{"Dollar exposure to USD changes QQQ returns", "Analyst raises QQQ price target after earnings", "Apple announces 2027 earnings forecast", "QQQ rises as treasury yields fall"} {
		if !p.Allows(InformationItem{Title: title}, nil) {
			t.Errorf("real news excluded: %s", title)
		}
	}
}

func TestNewsQualityPresetsAndInvalidRelevance(t *testing.T) {
	p := NewsPreferences{Keyword: "earnings", HiddenSources: []string{"noise"}}
	for _, want := range []string{"기본", "엄격", "전체"} {
		p = p.NextQualityMode()
		if p.QualityMode() != want || p.Keyword != "earnings" || len(p.HiddenSources) != 1 {
			t.Fatalf("preset destroyed independent settings: %+v", p)
		}
	}
	p = NewsPreferences{MinRelevance: 0.6}
	for _, score := range []string{"", "NaN", "Inf", "2", "-1", "invalid"} {
		if !p.Allows(InformationItem{Relevance: score}, nil) {
			t.Fatalf("invalid relevance filtered: %s", score)
		}
	}
	if !p.Allows(InformationItem{Relevance: "0.6"}, nil) || p.Allows(InformationItem{Relevance: "0.5999"}, nil) {
		t.Fatal("threshold boundary incorrect")
	}
}
