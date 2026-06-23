// Package discovery grows the company catalog automatically. Sources produce
// candidate boards (fuzzy, may be wrong); the pipeline dedupes against the
// catalog, confirms each candidate against the live ATS API, and adds the ones
// that have real postings. Because confirmation is the gate, sources can be as
// noisy as they like without polluting the catalog.
package discovery

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools/ats"
)

const (
	discoveryInterval  = 6 * time.Hour
	maxConfirmPerRun   = 300 // cap confirmations per run (politeness + bounded cost)
	confirmConcurrency = 8
	initialDelay       = 15 * time.Second
)

// Candidate is a possible new company board. ATS may be empty ("unknown"), in
// which case the pipeline tries all platforms.
type Candidate struct {
	ATS  string
	Slug string
	Name string
}

// Source yields candidate boards to consider adding.
type Source interface {
	Name() string
	Find(ctx context.Context) ([]Candidate, error)
}

type Discoverer struct {
	store   *store.Store
	sources []Source
}

func New(s *store.Store, sources ...Source) *Discoverer {
	return &Discoverer{store: s, sources: sources}
}

// Start runs an initial pass shortly after boot, then every interval.
func (d *Discoverer) Start() {
	go func() {
		time.Sleep(initialDelay)
		d.run()
		ticker := time.NewTicker(discoveryInterval)
		for range ticker.C {
			d.run()
		}
	}()
	slog.Info("discoverer started", "interval", discoveryInterval, "sources", len(d.sources))
}

func (d *Discoverer) run() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	existing, err := d.store.ListCompanyKeys(ctx)
	if err != nil {
		slog.Error("discoverer: load catalog keys", "error", err)
		return
	}

	var candidates []Candidate
	for _, src := range d.sources {
		found, err := src.Find(ctx)
		if err != nil {
			slog.Warn("discoverer: source failed", "source", src.Name(), "error", err)
			continue
		}
		slog.Info("discoverer: source candidates", "source", src.Name(), "count", len(found))
		candidates = append(candidates, found...)
	}

	candidates = dedupe(candidates, existing)

	// Shuffle then cap, so across runs we cover different candidates rather than
	// re-probing the same failing prefix forever.
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	if len(candidates) > maxConfirmPerRun {
		candidates = candidates[:maxConfirmPerRun]
	}

	added := d.confirmAndAdd(ctx, candidates)
	slog.Info("discoverer: run complete", "checked", len(candidates), "added", added)
}

// dedupe removes candidates already in the catalog and collapses duplicates.
func dedupe(cands []Candidate, existing map[[2]string]bool) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Slug == "" {
			continue
		}
		key := c.ATS + "|" + c.Slug
		if seen[key] {
			continue
		}
		seen[key] = true

		if c.ATS != "" {
			if existing[[2]string{c.ATS, c.Slug}] {
				continue
			}
		} else if existing[[2]string{ats.Greenhouse, c.Slug}] ||
			existing[[2]string{ats.Lever, c.Slug}] ||
			existing[[2]string{ats.Ashby, c.Slug}] {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (d *Discoverer) confirmAndAdd(ctx context.Context, cands []Candidate) int {
	sem := make(chan struct{}, confirmConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	added := 0

	for _, c := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func(c Candidate) {
			defer wg.Done()
			defer func() { <-sem }()

			resolvedATS, ok := d.confirm(ctx, c)
			if !ok {
				return
			}

			name := c.Name
			if name == "" {
				name = c.Slug
			}
			id, _ := uuid.NewV7()
			company := &models.Company{
				CompanyID: id.String(),
				Name:      name,
				ATS:       resolvedATS,
				Slug:      c.Slug,
			}
			if err := d.store.UpsertCompany(ctx, company); err != nil {
				slog.Warn("discoverer: upsert failed", "slug", c.Slug, "error", err)
				return
			}
			mu.Lock()
			added++
			mu.Unlock()
			slog.Info("discoverer: added company", "name", name, "ats", resolvedATS, "slug", c.Slug)
		}(c)
	}
	wg.Wait()
	return added
}

// confirm probes the candidate's board(s); returns the resolved ATS if it has
// postings. For unknown-ATS candidates it tries each platform.
func (d *Discoverer) confirm(ctx context.Context, c Candidate) (string, bool) {
	tryATS := []string{c.ATS}
	if c.ATS == "" {
		tryATS = []string{ats.Greenhouse, ats.Lever, ats.Ashby}
	}
	for _, a := range tryATS {
		if n, err := ats.Probe(ctx, a, c.Slug); err == nil && n > 0 {
			return a, true
		}
	}
	return "", false
}
