package click

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event represents a click on a shortlink.
type Event struct {
	LinkID    uuid.UUID
	ClickedAt time.Time
}

// DailyClicks represents clicks aggregated by day in UTC.
type DailyClicks struct {
	Date   string `json:"date"` // format YYYY-MM-DD
	Clicks int64  `json:"clicks"`
}

// Stats represents overall and daily stats for a link.
type Stats struct {
	TotalClicks  int64         `json:"total_clicks"`
	ClicksPerDay []DailyClicks `json:"clicks_per_day"`
}

// Repository defines data access for recording and querying clicks.
type Repository interface {
	InsertClicks(ctx context.Context, events []Event) error
	CountTotal(ctx context.Context, linkID uuid.UUID) (int64, error)
	ClicksPerDay(ctx context.Context, linkID uuid.UUID, since time.Time) ([]DailyClicks, error)
}
