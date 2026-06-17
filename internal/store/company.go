package store

import (
	"context"

	"github.com/gautamsardana/relay/internal/models"
)

func (s *Store) ListActiveCompanies(ctx context.Context) ([]models.Company, error) {
	rows, err := s.queries.ListActiveCompanies(ctx)
	if err != nil {
		return nil, err
	}
	companies := make([]models.Company, 0, len(rows))
	for _, row := range rows {
		r := row
		companies = append(companies, toModelCompany(&r))
	}
	return companies, nil
}

func (s *Store) UpsertCompany(ctx context.Context, company *models.Company) error {
	return s.queries.UpsertCompany(ctx, fromModelCompanyUpsert(company))
}
