package click

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LinkOwnershipChecker checks if a link with given code belongs to userID.
type LinkOwnershipChecker interface {
	CheckOwnership(ctx context.Context, code string, userID uuid.UUID) (uuid.UUID, error)
}

// Service provides click stats business logic.
type Service struct {
	repo    Repository
	checker LinkOwnershipChecker
	now     func() time.Time
}

// NewService creates a new Service.
func NewService(repo Repository, checker LinkOwnershipChecker, now func() time.Time) *Service {
	return &Service{
		repo:    repo,
		checker: checker,
		now:     now,
	}
}

// GetStats returns total and 30-day stats for the owned link code.
// If the link does not belong to the user, ErrNotFound is returned by the checker.
func (s *Service) GetStats(ctx context.Context, userID uuid.UUID, code string) (Stats, error) {
	linkID, err := s.checker.CheckOwnership(ctx, code, userID)
	if err != nil {
		return Stats{}, err
	}

	total, err := s.repo.CountTotal(ctx, linkID)
	if err != nil {
		return Stats{}, err
	}

	since := s.now().UTC().Add(-30 * 24 * time.Hour)
	daily, err := s.repo.ClicksPerDay(ctx, linkID, since)
	if err != nil {
		return Stats{}, err
	}

	return Stats{
		TotalClicks:  total,
		ClicksPerDay: daily,
	}, nil
}
