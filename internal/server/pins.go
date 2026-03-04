package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/wesm/agentsview/internal/db"
)

type pinRequest struct {
	Ordinal int     `json:"ordinal"`
	Note    *string `json:"note,omitempty"`
}

func (s *Server) handlePinMessage(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")
	messageIDStr := r.PathValue("messageId")
	messageID, err := strconv.ParseInt(messageIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}

	var req pinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	id, err := s.db.PinMessage(sessionID, messageID, req.Ordinal, req.Note)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleUnpinMessage(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")
	messageIDStr := r.PathValue("messageId")
	messageID, err := strconv.ParseInt(messageIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return
	}

	if err := s.db.UnpinMessage(sessionID, messageID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListPins(
	w http.ResponseWriter, r *http.Request,
) {
	pins, err := s.db.ListPinnedMessages(r.Context(), "")
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pins == nil {
		pins = []db.PinnedMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins})
}

func (s *Server) handleListSessionPins(
	w http.ResponseWriter, r *http.Request,
) {
	sessionID := r.PathValue("id")
	pins, err := s.db.ListPinnedMessages(r.Context(), sessionID)
	if err != nil {
		if handleContextError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pins == nil {
		pins = []db.PinnedMessage{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pins": pins})
}
