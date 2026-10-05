package config

import "testing"

func TestScannerSettingsValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Scanner)
	}{
		{"market", func(s *Scanner) { s.Market = "nxt" }},
		{"refresh", func(s *Scanner) { s.RefreshRaw = "5s" }},
		{"limit", func(s *Scanner) { s.MaxCandidates = 0 }},
		{"number", func(s *Scanner) { s.MinVolumeRatio = "invalid" }},
		{"negative", func(s *Scanner) { s.MinFiveMinuteRate = "-1" }},
		{"turnover", func(s *Scanner) { s.MinTurnoverKRW = 0 }},
	}
	defaults := Default().Scanner
	if err := defaults.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Default().Scanner
			test.edit(&s)
			if s.Validate() == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
