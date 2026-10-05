package sqlite

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func validArticleID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 32
}

func (r *Repository) InformationReadStates(ctx context.Context, ids []string) (map[string]time.Time, error) {
	result := map[string]time.Time{}
	for start := 0; start < len(ids); start += 200 {
		batch := ids[start:min(len(ids), start+200)]
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		marks := strings.TrimRight(strings.Repeat("?,", len(batch)), ",")
		rows, err := r.db.QueryContext(ctx, "SELECT article_id,read_at FROM information_reads WHERE article_id IN ("+marks+")", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, value string
			if err := rows.Scan(&id, &value); err != nil {
				rows.Close()
				return nil, err
			}
			stamp, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				rows.Close()
				return nil, err
			}
			result[id] = stamp
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) SetInformationRead(ctx context.Context, id string, read bool) (time.Time, error) {
	if !validArticleID(id) {
		return time.Time{}, fmt.Errorf("invalid article identifier")
	}
	if !read {
		_, err := r.db.ExecContext(ctx, "DELETE FROM information_reads WHERE article_id=?", id)
		return time.Time{}, err
	}
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `INSERT INTO information_reads(article_id,read_at) VALUES(?,?) ON CONFLICT(article_id) DO UPDATE SET read_at=excluded.read_at`, id, now.Format(time.RFC3339Nano))
	return now, err
}
