package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Request string `json:"request"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    workflowID, err := s.planner.CreateWorkflow(r.Context(), req.Request)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    json.NewEncoder(w).Encode(map[string]string{"workflow_id": workflowID})
}

func (s *Server) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	return 
}

func (s *Server) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	return 
}



