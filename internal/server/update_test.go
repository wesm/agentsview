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
	// Dev builds should not report update_available=true
	// even though CheckForUpdate returns info for display.
	if resp.UpdateAvailable {
		t.Error(
			"expected update_available=false for dev build",
		)
	}
	if !resp.IsDevBuild {
		t.Error("expected is_dev_build=true for dev build")
	}
}

func TestCheckUpdateEndpointOldVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: requires network access")
	}
	t.Parallel()

	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v0.0.1",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithDataDir(t.TempDir()),
	})

	w := te.get(t, "/api/v1/update/check")
	assertStatus(t, w, 200)

	resp := decode[updateCheckResp](t, w)
	if resp.CurrentVersion == "" {
		t.Error("current_version should not be empty")
	}
	// The endpoint hits the real GitHub API.
	// If a published release exists, v0.0.1 should report
	// an available update. If the API call fails (network
	// issues, no releases), the handler returns a graceful
	// degradation response with just current_version.
	if resp.UpdateAvailable {
		if resp.LatestVersion == "" {
			t.Error(
				"latest_version should be set when " +
					"update_available is true",
			)
		}
	}
}

func TestCheckUpdateEndpointError(t *testing.T) {
	t.Parallel()

	// Use a read-only directory as dataDir; the cache write
	// will fail, but the handler should still return 200 with
	// the current version.
	te := setupWithServerOpts(t, []server.Option{
		server.WithVersion(server.VersionInfo{
			Version:   "v99.99.99",
			Commit:    "abc123",
			BuildDate: "2026-01-01",
		}),
		server.WithDataDir("/dev/null/nonexistent"),
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
	if resp.UpdateAvailable {
		t.Error(
			"expected update_available=false for " +
				"error/degraded response",
		)
	}
}

type updateCheckResp struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	IsDevBuild      bool   `json:"is_dev_build"`
}
