package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func ListenAndServe(){
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/workflows", CreateWorkflow)
	r.Get("/workflows", ListWorkflows)
	r.Get("/workflows/{id}", GetWorkflow)

	http.ListenAndServe(":8080", r)
}