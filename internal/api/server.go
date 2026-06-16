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

	r.Post("/workers", s.CreateWorker)
	r.Get("/workers", s.ListWorkers)
	r.Get("/workers/{id}", s.GetWorker)
	r.Post("/workers/{id}/run", s.CreateRun)

	r.Get("/runs/{id}", s.GetRun)
	r.Get("/ws/runs/{id}", s.StreamRunSteps)

	// Serve static UI from /web at the root path.
	r.Handle("/*", http.FileServer(http.Dir("./web")))

	slog.Info("listening on port 8080...")
	http.ListenAndServe(":8080", r)
}
