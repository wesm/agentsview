package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/summary"
)

var validSummaryTypes = map[string]bool{
	"daily_activity": true,
	"agent_analysis": true,
}

func (s *Server) handleListSummaries(
	w http.ResponseWriter, r *http.Request,
) {
	q := r.URL.Query()

	typ := q.Get("type")
	if typ != "" && !validSummaryTypes[typ] {
		writeError(w, http.StatusBadRequest,
			"invalid type: must be daily_activity or agent_analysis")
		return
	}

	date := q.Get("date")
	if date != "" && !isValidDate(date) {
		writeError(w, http.StatusBadRequest,
			"invalid date format: use YYYY-MM-DD")
		return
	}

	filter := db.SummaryFilter{
		Type:    typ,
		Date:    date,
		Project: q.Get("project"),
	}

	summaries, err := s.db.ListSummaries(
		r.Context(), filter,
	)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}
	if summaries == nil {
		summaries = []db.Summary{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summaries": summaries,
	})
}

func (s *Server) handleGetSummary(
	w http.ResponseWriter, r *http.Request,
) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := s.db.GetSummary(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}
	if result == nil {
		writeError(w, http.StatusNotFound, "summary not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

type generateSummaryRequest struct {
	Type    string `json:"type"`
	Date    string `json:"date"`
	Project string `json:"project"`
	Prompt  string `json:"prompt"`
	Agent   string `json:"agent"`
}

func (s *Server) handleGenerateSummary(
	w http.ResponseWriter, r *http.Request,
) {
	var req generateSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid JSON body")
		return
	}

	if !validSummaryTypes[req.Type] {
		writeError(w, http.StatusBadRequest,
			"invalid type: must be daily_activity or agent_analysis")
		return
	}
	if !isValidDate(req.Date) {
		writeError(w, http.StatusBadRequest,
			"invalid date format: use YYYY-MM-DD")
		return
	}

	if req.Agent == "" {
		req.Agent = "claude"
	}
	if !summary.ValidAgents[req.Agent] {
		writeError(w, http.StatusBadRequest,
			"invalid agent: must be claude, codex, or gemini")
		return
	}

	stream, err := NewSSEStream(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError,
			"streaming not supported")
		return
	}

	stream.SendJSON("status", map[string]string{
		"phase": "generating",
	})

	prompt, err := summary.BuildPrompt(
		r.Context(), s.db, summary.GenerateRequest{
			Type:    req.Type,
			Date:    req.Date,
			Project: req.Project,
			Prompt:  req.Prompt,
		},
	)
	if err != nil {
		stream.SendJSON("error", map[string]string{
			"message": err.Error(),
		})
		return
	}

	genCtx, cancel := context.WithTimeout(
		r.Context(), 10*time.Minute,
	)
	defer cancel()

	result, err := summary.Generate(
		genCtx, req.Agent, prompt,
	)
	if err != nil {
		stream.SendJSON("error", map[string]string{
			"message": err.Error(),
		})
		return
	}

	if strings.TrimSpace(result.Content) == "" {
		stream.SendJSON("error", map[string]string{
			"message": "agent returned empty content",
		})
		return
	}

	var project *string
	if req.Project != "" {
		project = &req.Project
	}
	var model *string
	if result.Model != "" {
		model = &result.Model
	}
	var promptPtr *string
	if req.Prompt != "" {
		promptPtr = &req.Prompt
	}

	id, err := s.db.InsertSummary(db.Summary{
		Type:    req.Type,
		Date:    req.Date,
		Project: project,
		Agent:   result.Agent,
		Model:   model,
		Prompt:  promptPtr,
		Content: result.Content,
	})
	if err != nil {
		stream.SendJSON("error", map[string]string{
			"message": err.Error(),
		})
		return
	}

	saved, err := s.db.GetSummary(r.Context(), id)
	if err != nil || saved == nil {
		stream.SendJSON("error", map[string]string{
			"message": "failed to retrieve saved summary",
		})
		return
	}

	stream.SendJSON("done", saved)
}
