package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleStarSession(
	w http.ResponseWriter, r *http.Request,
) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	session, err := s.db.GetSession(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	if err := s.db.StarSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnstarSession(
	w http.ResponseWriter, r *http.Request,
) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	if err := s.db.UnstarSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListStarred(
	w http.ResponseWriter, r *http.Request,
) {
	ids, err := s.db.ListStarredSessionIDs(r.Context())
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_ids": ids,
	})
}

func (s *Server) handleBulkStar(
	w http.ResponseWriter, r *http.Request,
) {
	var body struct {
		SessionIDs []string `json:"session_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.SessionIDs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.db.BulkStarSessions(body.SessionIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
