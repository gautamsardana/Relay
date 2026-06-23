package api

import (
	"log/slog"
	"net/http"

	"github.com/gautamsardana/relay/internal/planner"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	planner *planner.Planner
}

func New(p *planner.Planner) *Server {
	return &Server{planner: p}
}

func (s *Server) ListenAndServe() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/users", s.CreateUser)
	r.Get("/users", s.GetUserByEmail)

	r.Post("/resumes/parse", s.ParseResume)

	r.Post("/workers", s.CreateWorker)
	r.Get("/workers", s.ListWorkers)
	r.Get("/workers/{id}", s.GetWorker)
	r.Patch("/workers/{id}/status", s.SetWorkerStatus)

	r.Post("/workers/{id}/run", s.CreateRun)
	r.Get("/runs/{id}", s.GetRun)
	r.Get("/ws/runs/{id}", s.StreamRunSteps)

	// Serve static UI from /web at the root path. no-cache so browsers always
	// pick up the latest JS/CSS during development (ES modules cache hard).
	r.Handle("/*", noCache(http.FileServer(http.Dir("./web"))))

	slog.Info("listening on port 8080...")
	http.ListenAndServe(":8080", r)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		h.ServeHTTP(w, r)
	})
}
