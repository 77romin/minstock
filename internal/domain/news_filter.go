package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type NewsPreferences struct {
	DirectMention bool     `json:"direct_mention"`
	MinRelevance  float64  `json:"min_relevance"`
	Keyword       string   `json:"keyword"`
	HiddenSources []string `json:"hidden_sources"`
	HideTemplates bool     `json:"hide_templates"`
}

func (p NewsPreferences) Allows(item InformationItem, symbols []Symbol) bool {
	return p.ExclusionReason(item, symbols) == ""
}

type NewsExclusion string

const (
	NewsExcludeKeyword   NewsExclusion = "검색"
	NewsExcludeSource    NewsExclusion = "매체"
	NewsExcludeTemplate  NewsExclusion = "반복형"
	NewsExcludeRelevance NewsExclusion = "관련도"
	NewsExcludeMention   NewsExclusion = "직접 언급"
)

// One primary reason per article makes exclusion counts additive and explainable.
func (p NewsPreferences) ExclusionReason(item InformationItem, symbols []Symbol) NewsExclusion {
	if p.Keyword != "" && !strings.Contains(strings.ToLower(item.Title), strings.ToLower(p.Keyword)) {
		return NewsExcludeKeyword
	}
	// Disclosures have no relevance scores and must never be hidden by the
	// news-only source/mention/score settings.
	if item.Kind == InformationDisclosure {
		return ""
	}
	for _, source := range p.HiddenSources {
		if strings.EqualFold(strings.TrimSpace(source), strings.TrimSpace(item.Source)) {
			return NewsExcludeSource
		}
	}
	if p.HideTemplates && templateNewsTitle(item.Title) {
		return NewsExcludeTemplate
	}
	if score, known := NewsRelevanceScore(item); p.MinRelevance > 0 && known && score < p.MinRelevance {
		return NewsExcludeRelevance
	}
	if !p.DirectMention {
		return ""
	}
	for _, symbol := range symbols {
		for _, code := range []string{symbol.Ticker, symbol.Code} {
			if code == "" {
				continue
			}
			if titleMentions(item.Title, code) {
				return ""
			}
		}
		if symbol.Name != "" {
			if titleMentions(item.Title, symbol.Name) {
				return ""
			}
		}
	}
	return NewsExcludeMention
}

func NewsRelevanceScore(item InformationItem) (float64, bool) {
	score, err := strconv.ParseFloat(strings.TrimSpace(item.Relevance), 64)
	return score, err == nil && !math.IsNaN(score) && !math.IsInf(score, 0) && score >= 0 && score <= 1
}

var currencyTemplate = regexp.MustCompile(`^\$?[A-Z][A-Z0-9.:-]{0,15}\s+(?i:to)\s+(?:KRW|USD|EUR|GBP|JPY|CNY|CAD|AUD|CHF|MXN|SGD|CLP|VND|INR|BRL|HKD|BTC|ETH)\b`)
var predictionTemplate = regexp.MustCompile(`(?i)\b(?:price prediction|price forecast|prediction\s*&\s*forecast)\s*(?:for\s+)?20\d{2}\b|(?:가격 예측|가격 전망)\s*20\d{2}년?`)
var currencyDescriptionTemplate = regexp.MustCompile(`(?i)\bstock price in (?:south korean won|mexican peso|singapore dollar|chilean peso|japanese yen)\b`)

func templateNewsTitle(title string) bool {
	title = InformationText(title)
	return currencyTemplate.MatchString(title) || predictionTemplate.MatchString(title) || currencyDescriptionTemplate.MatchString(title)
}

var koreanNewsParticles = []string{"으로부터", "에서부터", "에게서", "에게는", "에서는", "으로는", "에는", "에서", "으로", "에게", "까지", "부터", "처럼", "보다", "라고", "이며", "이고", "이나", "은", "는", "이", "가", "을", "를", "의", "에", "와", "과", "도", "로", "만"}

func newsWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }

// Match aliases literally rather than compiling a regex for every item/symbol.
// Korean particles are allowed, but prefixes/other share classes remain distinct.
func titleMentions(title, alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false
	}
	lower, needle := strings.ToLower(title), strings.ToLower(alias)
	short := len(alias) <= 2 && strings.IndexFunc(alias, func(r rune) bool { return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) }) < 0
	var originalRunes []rune
	if short {
		originalRunes = []rune(title)
	}
	runeOffset := 0
	for offset := 0; offset < len(lower); {
		rel := strings.Index(lower[offset:], needle)
		if rel < 0 {
			break
		}
		start, end := offset+rel, offset+rel+len(needle)
		runeStart := runeOffset
		if short {
			runeStart += utf8.RuneCountInString(lower[offset:start])
		}
		prefix, suffix := lower[:start], lower[end:]
		before, _ := utf8.DecodeLastRuneInString(prefix)
		after, _ := utf8.DecodeRuneInString(suffix)
		left := prefix == "" || !newsWordRune(before)
		right := suffix == "" || !newsWordRune(after)
		if !right && strings.IndexFunc(alias, func(r rune) bool { return r >= '가' && r <= '힣' }) >= 0 {
			for _, particle := range koreanNewsParticles {
				if strings.HasPrefix(suffix, particle) {
					rest := strings.TrimPrefix(suffix, particle)
					next, _ := utf8.DecodeRuneInString(rest)
					if rest == "" || !newsWordRune(next) {
						right = true
						break
					}
				}
			}
		}
		if left && right {
			if !short {
				return true
			}
			// A / V / T are common words; require $A, (A), or EXCHANGE:A.
			exchange := false
			if before == ':' {
				for _, name := range []string{"nasdaq:", "nyse:", "amex:", "nysearca:", "bats:"} {
					if strings.HasSuffix(prefix, name) {
						exchange = true
						break
					}
				}
			}
			explicit := before == '$' || (before == '(' && after == ')') || exchange
			if explicit || (len(alias) == 2 && string(originalRunes[runeStart:runeStart+2]) == strings.ToUpper(alias)) {
				return true
			}
		}
		if short {
			runeOffset = runeStart + len(alias)
		}
		offset = end
	}
	return false
}

func (p NewsPreferences) QualityMode() string {
	switch {
	case !p.DirectMention && p.MinRelevance == 0 && !p.HideTemplates:
		return "전체"
	case !p.DirectMention && p.MinRelevance == 0.3 && p.HideTemplates:
		return "기본"
	case p.DirectMention && p.MinRelevance == 0.6 && p.HideTemplates:
		return "엄격"
	default:
		return "사용자"
	}
}

func (p NewsPreferences) NextQualityMode() NewsPreferences {
	switch p.QualityMode() {
	case "전체":
		p.DirectMention, p.MinRelevance, p.HideTemplates = false, 0.3, true
	case "기본":
		p.DirectMention, p.MinRelevance, p.HideTemplates = true, 0.6, true
	default:
		p.DirectMention, p.MinRelevance, p.HideTemplates = false, 0, false
	}
	return p
}

func (p NewsPreferences) Normalized() NewsPreferences {
	if p.MinRelevance != 0 && p.MinRelevance != 0.3 && p.MinRelevance != 0.6 {
		p.MinRelevance = 0
	}
	p.Keyword = InformationText(p.Keyword)
	if len([]rune(p.Keyword)) > 80 {
		p.Keyword = string([]rune(p.Keyword)[:80])
	}
	seen := map[string]bool{}
	sources := make([]string, 0, len(p.HiddenSources))
	for _, source := range p.HiddenSources {
		source = InformationText(source)
		key := strings.ToLower(source)
		if source != "" && !seen[key] && len(sources) < 100 {
			sources = append(sources, source)
			seen[key] = true
		}
	}
	p.HiddenSources = sources
	return p
}
