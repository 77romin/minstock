package domain

import (
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

type InformationKind string

const (
	InformationNews       InformationKind = "NEWS"
	InformationDisclosure InformationKind = "DISCLOSURE"
)

// InformationItem stores metadata and a source link, never article bodies.
type InformationItem struct {
	Kind        InformationKind
	Title       string
	URL         string
	Source      string
	PublishedAt time.Time
	DateOnly    bool
	Relevance   string
}

var informationTags = regexp.MustCompile(`<[^>]*>`)
var terminalSequences = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)

func InformationText(value string) string {
	value = terminalSequences.ReplaceAllString(html.UnescapeString(value), "")
	value = informationTags.ReplaceAllString(value, "")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func InformationURL(raw string) string {
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	query := u.Query()
	for key := range query {
		if strings.HasPrefix(strings.ToLower(key), "utm_") || key == "fbclid" || key == "gclid" {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func NormalizeInformation(items []InformationItem) []InformationItem {
	items = append([]InformationItem(nil), items...)
	result := make([]InformationItem, 0, len(items))
	urls, titles := map[string]bool{}, map[string]bool{}
	sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(items[j].PublishedAt) })
	for _, item := range items {
		item.Title, item.Source, item.Relevance = InformationText(item.Title), InformationText(item.Source), InformationText(item.Relevance)
		item.URL = InformationURL(item.URL)
		if item.Title == "" || item.URL == "" || item.PublishedAt.IsZero() {
			continue
		}
		// Corrected disclosures must remain distinct; news syndication is grouped
		// by title within a publication date as well as by canonical URL.
		titleKey := string(item.Kind) + ":" + item.PublishedAt.Format("2006-01-02") + ":" + strings.ToLower(item.Title)
		if urls[item.URL] || (item.Kind == InformationNews && titles[titleKey]) {
			continue
		}
		urls[item.URL], titles[titleKey] = true, true
		result = append(result, item)
	}
	return result
}
