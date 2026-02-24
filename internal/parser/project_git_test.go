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

func TestExtractProjectFromCwdWithBranch_OfflineWorktreePath(t *testing.T) {
	cwd := "/Users/wesm/code/agentsview-worktree-tool-call-arguments"
	got := ExtractProjectFromCwdWithBranch(
		cwd, "worktree-tool-call-arguments",
	)
	if got != "agentsview" {
		t.Fatalf("ExtractProjectFromCwdWithBranch = %q, want %q", got, "agentsview")
	}
}

func TestExtractProjectFromCwdWithBranch_BranchWithSlash(t *testing.T) {
	cwd := "/Users/wesm/code/agentsview-feature-worktree-support"
	got := ExtractProjectFromCwdWithBranch(
		cwd, "feature/worktree-support",
	)
	if got != "agentsview" {
		t.Fatalf("ExtractProjectFromCwdWithBranch = %q, want %q", got, "agentsview")
	}
}

func TestExtractProjectFromCwdWithBranch_MismatchNoTrim(t *testing.T) {
	cwd := "/Users/wesm/code/agentsview-hotfix"
	got := ExtractProjectFromCwdWithBranch(cwd, "feature/other")
	if got != "agentsview_hotfix" {
		t.Fatalf("ExtractProjectFromCwdWithBranch = %q, want %q", got, "agentsview_hotfix")
	}
}

func TestExtractProjectFromCwdWithBranch_DefaultBranchNoTrim(t *testing.T) {
	cwd := "/Users/wesm/code/project-main"
	got := ExtractProjectFromCwdWithBranch(cwd, "main")
	if got != "project_main" {
		t.Fatalf("ExtractProjectFromCwdWithBranch = %q, want %q", got, "project_main")
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
