package link

import (
	"context"

	"github.com/google/uuid"
)

// CheckOwnership returns the link ID if the link belongs to userID, or ErrNotFound otherwise.
func (s *Service) CheckOwnership(ctx context.Context, code string, userID uuid.UUID) (uuid.UUID, error) {
	l, err := s.repo.GetOwned(ctx, code, userID)
	if err != nil {
		return uuid.Nil, err
	}
	return l.ID, nil
}
