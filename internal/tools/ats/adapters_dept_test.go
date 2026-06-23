package ats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGreenhouseDepartment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":1,"title":"Backend Engineer","absolute_url":"u","first_published":"2026-06-01T00:00:00Z","departments":[{"name":"Engineering"}],"location":{"name":"Remote"}}]}`))
	}))
	defer srv.Close()
	a := &greenhouseAdapter{client: srv.Client(), baseURL: srv.URL}
	jobs, err := a.Fetch(context.Background(), "x", "X")
	if err != nil || len(jobs) != 1 || jobs[0].Department != "Engineering" {
		t.Fatalf("got %+v err=%v", jobs, err)
	}
}

func TestLeverDepartmentTeam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"x","text":"SWE","hostedUrl":"u","createdAt":1700000000000,"descriptionPlain":"d","categories":{"location":"Remote","department":"Eng","team":"Platform"}}]`))
	}))
	defer srv.Close()
	a := &leverAdapter{client: srv.Client(), baseURL: srv.URL}
	jobs, err := a.Fetch(context.Background(), "x", "X")
	if err != nil || len(jobs) != 1 || jobs[0].Department != "Eng" || jobs[0].Team != "Platform" {
		t.Fatalf("got %+v err=%v", jobs, err)
	}
}

func TestAshbyDepartmentTeam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":"x","title":"SWE","location":"Remote","jobUrl":"u","publishedAt":"2026-06-01T00:00:00Z","department":"Product","team":"Engineering","descriptionPlain":"d"}]}`))
	}))
	defer srv.Close()
	a := &ashbyAdapter{client: srv.Client(), baseURL: srv.URL}
	jobs, err := a.Fetch(context.Background(), "x", "X")
	if err != nil || len(jobs) != 1 || jobs[0].Department != "Product" || jobs[0].Team != "Engineering" {
		t.Fatalf("got %+v err=%v", jobs, err)
	}
}
