package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/77romin/minstock-tui/internal/domain"
)

const newsPreferencesKey = "news-preferences:v1"

func (s *Service) NewsPreferences(ctx context.Context) (domain.NewsPreferences, error) {
	data, _, err := s.repo.LoadCache(ctx, newsPreferencesKey)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsPreferences{}, nil
	}
	if err != nil {
		return domain.NewsPreferences{}, err
	}
	var p domain.NewsPreferences
	err = json.Unmarshal(data, &p)
	return p.Normalized(), err
}

func (s *Service) SaveNewsPreferences(ctx context.Context, p domain.NewsPreferences) error {
	data, err := json.Marshal(p.Normalized())
	if err != nil {
		return err
	}
	return s.repo.SaveCache(ctx, newsPreferencesKey, data)
}
