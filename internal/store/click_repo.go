package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kyyril/shortlink/internal/click"
	"github.com/kyyril/shortlink/internal/store/sqlc"
)

// ClickRepo implements click.Repository using pgxpool and generated sqlc queries.
type ClickRepo struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewClickRepo creates a new ClickRepo.
func NewClickRepo(pool *pgxpool.Pool) *ClickRepo {
	return &ClickRepo{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// InsertClicks batch inserts click events using PostgreSQL COPY protocol.
func (r *ClickRepo) InsertClicks(ctx context.Context, events []click.Event) error {
	if len(events) == 0 {
		return nil
	}

	params := make([]sqlc.InsertClicksParams, len(events))
	for i, e := range events {
		params[i] = sqlc.InsertClicksParams{
			LinkID:    e.LinkID,
			ClickedAt: e.ClickedAt,
		}
	}

	_, err := r.queries.InsertClicks(ctx, params)
	if err != nil {
		return fmt.Errorf("insert clicks batch: %w", err)
	}

	return nil
}

// CountTotal returns total clicks for a link.
func (r *ClickRepo) CountTotal(ctx context.Context, linkID uuid.UUID) (int64, error) {
	total, err := r.queries.CountTotalClicks(ctx, linkID)
	if err != nil {
		return 0, fmt.Errorf("count total clicks: %w", err)
	}
	return total, nil
}

// ClicksPerDay returns clicks grouped by day for dates >= since. Days without clicks are omitted.
func (r *ClickRepo) ClicksPerDay(ctx context.Context, linkID uuid.UUID, since time.Time) ([]click.DailyClicks, error) {
	rows, err := r.queries.ClicksPerDay(ctx, sqlc.ClicksPerDayParams{
		LinkID: linkID,
		Since:  since,
	})
	if err != nil {
		return nil, fmt.Errorf("clicks per day: %w", err)
	}

	result := make([]click.DailyClicks, len(rows))
	for i, row := range rows {
		dateStr := ""
		if row.Day.Valid {
			dateStr = row.Day.Time.Format("2006-01-02")
		}
		result[i] = click.DailyClicks{
			Date:   dateStr,
			Clicks: row.Clicks,
		}
	}

	return result, nil
}
