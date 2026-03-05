package server

import (
	"net/http"

	"github.com/wesm/agentsview/internal/update"
)

type updateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	IsDevBuild      bool   `json:"is_dev_build,omitempty"`
}

func (s *Server) handleCheckUpdate(
	w http.ResponseWriter, _ *http.Request,
) {
	info, err := update.CheckForUpdate(
		s.version.Version, false, s.dataDir,
	)
	if err != nil {
		writeJSON(w, http.StatusOK, updateCheckResponse{
			CurrentVersion: s.version.Version,
		})
		return
	}

	if info == nil {
		writeJSON(w, http.StatusOK, updateCheckResponse{
			CurrentVersion: s.version.Version,
		})
		return
	}

	writeJSON(w, http.StatusOK, updateCheckResponse{
		UpdateAvailable: !info.IsDevBuild,
		CurrentVersion:  info.CurrentVersion,
		LatestVersion:   info.LatestVersion,
		IsDevBuild:      info.IsDevBuild,
	})
}
