package planner

import (
	"context"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
)

func (p *Planner) CreateUser(ctx context.Context, email string) (models.User, error) {
	id, _ := uuid.NewV7()

	user := &models.User{
		UserID: id.String(),
		Email:  email,
	}

	return p.store.CreateUser(ctx, user)
}

func (p *Planner) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	return p.store.GetUserByID(ctx, userID)
}

func (p *Planner) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	return p.store.GetUserByEmail(ctx, email)
}
