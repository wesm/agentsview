package server_test

import (
	"errors"
	"testing"

	"github.com/wesm/agentsview/internal/server"
	"github.com/wesm/agentsview/internal/update"
)

func stubChecker(
	info *update.UpdateInfo, err error,
) server.UpdateCheckFunc {
	return func(string, bool, string) (*update.UpdateInfo, error) {
		return info, err
	}
}

func TestCheckUpdateUpToDate(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v1.0.0",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithUpdateChecker(stubChecker(nil, nil)),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.CurrentVersion != "v1.0.0" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "v1.0.0",
		)
	}
	if resp.UpdateAvailable {
		t.Error("expected update_available=false when up to date")
	}
}

func TestCheckUpdateAvailable(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v0.9.0",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithUpdateChecker(stubChecker(
			&update.UpdateInfo{
				CurrentVersion: "v0.9.0",
				LatestVersion:  "v1.0.0",
			},
			nil,
		)),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if !resp.UpdateAvailable {
		t.Error("expected update_available=true")
	}
	if resp.LatestVersion != "v1.0.0" {
		t.Errorf(
			"latest_version = %q, want %q",
			resp.LatestVersion, "v1.0.0",
		)
	}
	if resp.CurrentVersion != "v0.9.0" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "v0.9.0",
		)
	}
}

func TestCheckUpdateDevBuild(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "dev",
			Commit:    "unknown",
			BuildDate: "",
		}),
		server.WithUpdateChecker(stubChecker(
			&update.UpdateInfo{
				CurrentVersion: "dev",
				LatestVersion:  "v1.0.0",
				IsDevBuild:     true,
			},
			nil,
		)),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.UpdateAvailable {
		t.Error(
			"expected update_available=false for dev build",
		)
	}
	if !resp.IsDevBuild {
		t.Error("expected is_dev_build=true")
	}
	if resp.CurrentVersion != "dev" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "dev",
		)
	}
}

func TestCheckUpdateError(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v1.0.0",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithUpdateChecker(stubChecker(
			nil, errors.New("network error"),
		)),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.CurrentVersion != "v1.0.0" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "v1.0.0",
		)
	}
	if resp.UpdateAvailable {
		t.Error(
			"expected update_available=false on error",
		)
	}
}

type updateCheckResp struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	IsDevBuild      bool   `json:"is_dev_build"`
}
