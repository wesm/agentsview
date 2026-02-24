package server

import (
	"net/http"
	"strings"

	"github.com/wesm/agentsview/internal/db"
)

type searchResponse struct {
	Query   string            `json:"query"`
	Results []db.SearchResult `json:"results"`
	Count   int               `json:"count"`
	Next    int               `json:"next"`
}

// prepareFTSQuery wraps multi-word queries in quotes so
// SQLite FTS matches the exact phrase rather than individual
// terms.
func prepareFTSQuery(raw string) string {
	if strings.Contains(raw, " ") &&
		!strings.HasPrefix(raw, "\"") {
		return "\"" + raw + "\""
	}
	return raw
}

func (s *Server) handleSearch(
	w http.ResponseWriter, r *http.Request,
) {
	q := r.URL.Query()

	query := strings.TrimSpace(q.Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}

	limit, ok := parseIntParam(w, r, "limit")
	if !ok {
		return
	}
	limit = clampLimit(limit, db.DefaultSearchLimit, db.MaxSearchLimit)

	cursor, ok := parseIntParam(w, r, "cursor")
	if !ok {
		return
	}

	if !s.db.HasFTS() {
		writeError(w, http.StatusNotImplemented, "search not available")
		return
	}

	filter := db.SearchFilter{
		Query:   prepareFTSQuery(query),
		Project: q.Get("project"),
		Cursor:  cursor,
		Limit:   limit,
	}

	page, err := s.db.Search(r.Context(), filter)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, searchResponse{
		Query:   query,
		Results: page.Results,
		Count:   len(page.Results),
		Next:    page.NextCursor,
	})
}
