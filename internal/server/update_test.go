package server_test

import (
	"testing"

	"github.com/wesm/agentsview/internal/server"
)

func TestCheckUpdateEndpoint(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v99.99.99",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithDataDir(t.TempDir()),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.CurrentVersion != "v99.99.99" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "v99.99.99",
		)
	}
	// Version v99.99.99 is higher than any real release,
	// so update_available should be false.
	if resp.UpdateAvailable {
		t.Error("expected update_available=false for unreleased version")
	}
}

func TestCheckUpdateEndpointDevBuild(t *testing.T) {
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "dev",
			Commit:    "unknown",
			BuildDate: "",
		}),
		server.WithDataDir(t.TempDir()),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.CurrentVersion != "dev" {
		t.Errorf(
			"current_version = %q, want %q",
			resp.CurrentVersion, "dev",
		)
	}
}

type updateCheckResp struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	IsDevBuild      bool   `json:"is_dev_build"`
}
