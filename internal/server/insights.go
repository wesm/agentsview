package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/insight"
)

var validInsightTypes = map[string]bool{
	"daily_activity": true,
	"agent_analysis": true,
}

func (s *Server) handleListInsights(
	w http.ResponseWriter, r *http.Request,
) {
	q := r.URL.Query()

	typ := q.Get("type")
	if typ != "" && !validInsightTypes[typ] {
		writeError(w, http.StatusBadRequest,
			"invalid type: must be daily_activity or agent_analysis")
		return
	}

	filter := db.InsightFilter{
		Type:    typ,
		Project: q.Get("project"),
	}

	insights, err := s.db.ListInsights(
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
	if insights == nil {
		insights = []db.Insight{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"insights": insights,
	})
}

func (s *Server) handleGetInsight(
	w http.ResponseWriter, r *http.Request,
) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := s.db.GetInsight(r.Context(), id)
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
		writeError(w, http.StatusNotFound, "insight not found")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDeleteInsight(
	w http.ResponseWriter, r *http.Request,
) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := s.db.GetInsight(r.Context(), id)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "insight not found")
		return
	}

	if err := s.db.DeleteInsight(id); err != nil {
		writeError(
			w, http.StatusInternalServerError, err.Error(),
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type generateInsightRequest struct {
	Type     string `json:"type"`
	DateFrom string `json:"date_from"`
	DateTo   string `json:"date_to"`
	Project  string `json:"project"`
	Prompt   string `json:"prompt"`
	Agent    string `json:"agent"`
}

func (s *Server) handleGenerateInsight(
	w http.ResponseWriter, r *http.Request,
) {
	var req generateInsightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest,
			"invalid JSON body")
		return
	}

	if !validInsightTypes[req.Type] {
		writeError(w, http.StatusBadRequest,
			"invalid type: must be daily_activity or agent_analysis")
		return
	}
	if !isValidDate(req.DateFrom) {
		writeError(w, http.StatusBadRequest,
			"invalid date_from: use YYYY-MM-DD")
		return
	}
	if !isValidDate(req.DateTo) {
		writeError(w, http.StatusBadRequest,
			"invalid date_to: use YYYY-MM-DD")
		return
	}
	if req.DateTo < req.DateFrom {
		writeError(w, http.StatusBadRequest,
			"date_to must be >= date_from")
		return
	}

	if req.Agent == "" {
		req.Agent = "claude"
	}
	if !insight.ValidAgents[req.Agent] {
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

	prompt, err := insight.BuildPrompt(
		r.Context(), s.db, insight.GenerateRequest{
			Type:     req.Type,
			DateFrom: req.DateFrom,
			DateTo:   req.DateTo,
			Project:  req.Project,
			Prompt:   req.Prompt,
		},
	)
	if err != nil {
		log.Printf("insight prompt error: %v", err)
		stream.SendJSON("error", map[string]string{
			"message": "failed to build prompt",
		})
		return
	}

	genCtx, cancel := context.WithTimeout(
		r.Context(), 10*time.Minute,
	)
	defer cancel()

	result, err := s.generateFunc(
		genCtx, req.Agent, prompt,
	)
	if err != nil {
		log.Printf("insight generate error: %v", err)
		stream.SendJSON("error", map[string]string{
			"message": fmt.Sprintf(
				"%s generation failed", req.Agent,
			),
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

	id, err := s.db.InsertInsight(db.Insight{
		Type:     req.Type,
		DateFrom: req.DateFrom,
		DateTo:   req.DateTo,
		Project:  project,
		Agent:    result.Agent,
		Model:    model,
		Prompt:   promptPtr,
		Content:  result.Content,
	})
	if err != nil {
		log.Printf("insight insert error: %v", err)
		stream.SendJSON("error", map[string]string{
			"message": "failed to save insight",
		})
		return
	}

	saved, err := s.db.GetInsight(r.Context(), id)
	if err != nil || saved == nil {
		log.Printf("insight get error: id=%d err=%v",
			id, err)
		stream.SendJSON("error", map[string]string{
			"message": "failed to retrieve saved insight",
		})
		return
	}

	stream.SendJSON("done", saved)
}
