package server

import (
	"net/http"

	"github.com/wesm/agentsview/internal/update"
)

// UpdateCheckFunc is the signature for functions that check for
// available updates. The default is update.CheckForUpdate.
type UpdateCheckFunc func(
	currentVersion string,
	forceCheck bool,
	cacheDir string,
) (*update.UpdateInfo, error)

type updateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	IsDevBuild      bool   `json:"is_dev_build,omitempty"`
}

func (s *Server) handleCheckUpdate(
	w http.ResponseWriter, _ *http.Request,
) {
	checkFn := s.updateCheckFn
	if checkFn == nil {
		checkFn = update.CheckForUpdate
	}

	info, err := checkFn(
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
