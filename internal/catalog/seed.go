package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
)

// companiesJSON is the checked-in starter catalog, baked into the binary at
// build time so there's no runtime file-path dependency.
//
//go:embed companies.json
var companiesJSON []byte

type seedCompany struct {
	Name string `json:"name"`
	ATS  string `json:"ats"`
	Slug string `json:"slug"`
}

// Seed upserts the checked-in company catalog into the DB. It is idempotent and
// safe to run on every startup: UpsertCompany only touches rows present in the
// JSON, so companies added later by other means are never wiped.
func Seed(ctx context.Context, s *store.Store) error {
	var companies []seedCompany
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return fmt.Errorf("catalog seed: failed to parse companies.json: %w", err)
	}

	for _, c := range companies {
		id, _ := uuid.NewV7()
		company := &models.Company{
			CompanyID: id.String(),
			Name:      c.Name,
			ATS:       c.ATS,
			Slug:      c.Slug,
		}
		if err := s.UpsertCompany(ctx, company); err != nil {
			return fmt.Errorf("catalog seed: failed to upsert %s/%s: %w", c.ATS, c.Slug, err)
		}
	}

	slog.Info("catalog seeded", "count", len(companies))
	return nil
}
