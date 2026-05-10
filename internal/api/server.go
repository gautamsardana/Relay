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

func (s *Server) ListenAndServe(){
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/workflow", s.CreateWorkflow)
	r.Get("/workflows", s.ListWorkflows)
	r.Get("/workflows/{id}", s.GetWorkflow)
	
	slog.Info("listening on port 8080...")
	http.ListenAndServe(":8080", r)
}