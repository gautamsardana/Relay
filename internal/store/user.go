package store

import (
	"context"

	"github.com/gautamsardana/relay/internal/models"
)

func (s *Store) CreateUser(ctx context.Context, user *models.User) (models.User, error) {
	row, err := s.queries.CreateUser(ctx, fromModelUserCreate(user))
	if err != nil {
		return models.User{}, err
	}
	return toModelUser(&row), nil
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	row, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return models.User{}, err
	}
	return toModelUser(&row), nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return models.User{}, err
	}
	return toModelUser(&row), nil
}
