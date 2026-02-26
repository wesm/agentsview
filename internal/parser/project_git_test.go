package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractProjectFromCwd_GitRepoRoot(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "my-app")
	subdir := filepath.Join(repo, "internal", "sync")

	mustMkdirAll(t, filepath.Join(repo, ".git"))
	mustMkdirAll(t, subdir)

	got := ExtractProjectFromCwd(subdir)
	if got != "my_app" {
		t.Fatalf("ExtractProjectFromCwd(%q) = %q, want %q", subdir, got, "my_app")
	}
}

func TestExtractProjectFromCwd_GitWorktree(t *testing.T) {
	root := t.TempDir()

	mainRepo := filepath.Join(root, "agentsview")
	worktree := filepath.Join(root, "agentsview-worktree-tool-calls")
	worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "feature")

	mustMkdirAll(t, filepath.Join(mainRepo, ".git"))
	mustMkdirAll(t, worktreeGitDir)
	mustMkdirAll(t, filepath.Join(worktree, "internal"))

	mustWriteFile(t, filepath.Join(worktree, ".git"),
		"gitdir: "+worktreeGitDir+"\n")
	// Matches git's linked-worktree layout.
	mustWriteFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")

	got := ExtractProjectFromCwd(filepath.Join(worktree, "internal"))
	if got != "agentsview" {
		t.Fatalf("ExtractProjectFromCwd(worktree) = %q, want %q", got, "agentsview")
	}
}

func TestExtractProjectFromCwd_GitWorktreeFallbackWithoutCommondir(t *testing.T) {
	root := t.TempDir()

	mainRepo := filepath.Join(root, "my-repo")
	worktree := filepath.Join(root, "my-repo-experiment")
	worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "exp")

	mustMkdirAll(t, filepath.Join(mainRepo, ".git"))
	mustMkdirAll(t, worktreeGitDir)
	mustMkdirAll(t, worktree)

	mustWriteFile(t, filepath.Join(worktree, ".git"),
		"gitdir: "+worktreeGitDir+"\n")

	got := ExtractProjectFromCwd(worktree)
	if got != "my_repo" {
		t.Fatalf("ExtractProjectFromCwd(worktree) = %q, want %q", got, "my_repo")
	}
}

func TestExtractProjectFromCwdWithBranch(t *testing.T) {
	tests := []struct {
		name   string
		cwd    string
		branch string
		want   string
	}{
		{
			name:   "OfflineWorktreePath",
			cwd:    filepath.FromSlash("/Users/wesm/code/agentsview-worktree-tool-call-arguments"),
			branch: "worktree-tool-call-arguments",
			want:   "agentsview",
		},
		{
			name:   "BranchWithSlash",
			cwd:    filepath.FromSlash("/Users/wesm/code/agentsview-feature-worktree-support"),
			branch: "feature/worktree-support",
			want:   "agentsview",
		},
		{
			name:   "MismatchNoTrim",
			cwd:    filepath.FromSlash("/Users/wesm/code/agentsview-hotfix"),
			branch: "feature/other",
			want:   "agentsview_hotfix",
		},
		{
			name:   "DefaultBranchNoTrim",
			cwd:    filepath.FromSlash("/Users/wesm/code/project-main"),
			branch: "main",
			want:   "project_main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProjectFromCwdWithBranch(tt.cwd, tt.branch)
			if got != tt.want {
				t.Fatalf("ExtractProjectFromCwdWithBranch(%q, %q) = %q, want %q", tt.cwd, tt.branch, got, tt.want)
			}
		})
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
