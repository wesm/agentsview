package parser

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var projectMarkers = []string{
	"code", "projects", "repos", "src", "work", "dev",
}

var ignoredSystemDirs = map[string]bool{
	"users": true, "home": true, "var": true,
	"tmp": true, "private": true,
}

func normalizeName(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

// GetProjectName converts an encoded Claude project directory name
// to a clean project name. Claude encodes paths like
// /Users/alice/code/my-app as -Users-alice-code-my-app.
func GetProjectName(dirName string) string {
	if dirName == "" {
		return ""
	}

	if !strings.HasPrefix(dirName, "-") {
		return normalizeName(dirName)
	}

	parts := strings.Split(dirName, "-")

	// Strategy 1: find a known project parent directory marker
	for _, marker := range projectMarkers {
		for i, part := range parts {
			if strings.EqualFold(part, marker) && i+1 < len(parts) {
				result := strings.Join(parts[i+1:], "-")
				if result != "" {
					return normalizeName(result)
				}
			}
		}
	}

	// Strategy 2: use last non-system-directory component
	for i := len(parts) - 1; i >= 0; i-- {
		if p := parts[i]; p != "" && !ignoredSystemDirs[strings.ToLower(p)] {
			return normalizeName(p)
		}
	}

	return normalizeName(dirName)
}

// ExtractProjectFromCwd extracts a project name from a working
// directory path. If cwd is inside a git repository (including
// linked worktrees), this returns the repository root directory
// name. Otherwise it falls back to the last path component.
func ExtractProjectFromCwd(cwd string) string {
	return ExtractProjectFromCwdWithBranch(cwd, "")
}

// ExtractProjectFromCwdWithBranch extracts a canonical project
// name from cwd and optionally git branch metadata. Branch is
// used as a fallback heuristic when the original worktree path no
// longer exists on disk.
func ExtractProjectFromCwdWithBranch(
	cwd, gitBranch string,
) string {
	if cwd == "" {
		return ""
	}
	cleaned := filepath.Clean(cwd)
	if root := findGitRepoRoot(cleaned); root != "" {
		name := filepath.Base(root)
		if isInvalidPathBase(name) {
			return ""
		}
		return normalizeName(name)
	}

	name := filepath.Base(cleaned)
	if isInvalidPathBase(name) {
		return ""
	}
	name = trimBranchSuffix(name, gitBranch)
	if isInvalidPathBase(name) {
		return ""
	}
	return normalizeName(name)
}

func isInvalidPathBase(name string) bool {
	if name == "." || name == ".." || name == "/" || name == string(filepath.Separator) {
		return true
	}
	if strings.ContainsAny(name, "/\\") {
		return true
	}
	return false
}

// findGitRepoRoot walks upward from cwd to find the enclosing git
// repository root. Supports both standard repos (.git directory)
// and linked worktrees/submodules (.git file).
func findGitRepoRoot(cwd string) string {
	if cwd == "" {
		return ""
	}

	dir := cwd
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			dir = filepath.Dir(dir)
		}
	} else {
		// Avoid treating non-path strings as cwd.
		if !strings.ContainsRune(dir, filepath.Separator) {
			return ""
		}
		dir = filepath.Dir(dir)
	}

	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return dir
			}
			if info.Mode().IsRegular() {
				if root := repoRootFromGitFile(dir, gitPath); root != "" {
					return root
				}
				// Keep conservative fallback for gitfile repos
				// when metadata cannot be parsed.
				return dir
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func repoRootFromGitFile(repoDir, gitFilePath string) string {
	gitDir := readGitDirFromFile(gitFilePath)
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Clean(
			filepath.Join(filepath.Dir(gitFilePath), gitDir),
		)
	}

	commonDir := readCommonDir(gitDir)
	if commonDir != "" {
		if filepath.Base(commonDir) == ".git" {
			return filepath.Dir(commonDir)
		}
	}

	// Fallback for linked worktrees if commondir is missing.
	marker := string(filepath.Separator) + ".git" +
		string(filepath.Separator) + "worktrees" +
		string(filepath.Separator)
	if root, _, found := strings.Cut(gitDir, marker); found {
		if root != "" {
			return filepath.Clean(root)
		}
	}

	return repoDir
}

func readGitDirFromFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		const prefix = "gitdir:"
		if strings.HasPrefix(strings.ToLower(line), prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}

func readCommonDir(gitDir string) string {
	b, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(b))
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(gitDir, value))
}

func trimBranchSuffix(name, gitBranch string) string {
	branch := strings.TrimSpace(gitBranch)
	if name == "" || branch == "" {
		return name
	}
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branchToken := normalizeBranchToken(branch)
	if branchToken == "" {
		return name
	}
	if isDefaultBranchToken(branchToken) {
		return name
	}

	for _, sep := range []string{"-", "_"} {
		suffix := sep + branchToken
		if strings.HasSuffix(
			strings.ToLower(name),
			strings.ToLower(suffix),
		) {
			base := strings.TrimRight(
				name[:len(name)-len(suffix)], "-_",
			)
			if base != "" {
				return base
			}
		}
	}
	return name
}

func normalizeBranchToken(branch string) string {
	var b strings.Builder
	b.Grow(len(branch))

	lastDash := false
	for _, r := range branch {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '/', r == '-', r == '_', r == '.', unicode.IsSpace(r):
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	return out
}

func isDefaultBranchToken(branch string) bool {
	switch strings.ToLower(strings.TrimSpace(branch)) {
	case "main", "master", "trunk", "develop", "dev":
		return true
	default:
		return false
	}
}

// NeedsProjectReparse checks if a stored project name looks like
// an un-decoded encoded path that should be re-extracted.
func NeedsProjectReparse(project string) bool {
	bad := []string{
		"_Users", "_home", "_private", "_tmp", "_var",
	}
	for _, prefix := range bad {
		if strings.HasPrefix(project, prefix) {
			return true
		}
	}
	return strings.Contains(project, "_var_folders_") ||
		strings.Contains(project, "_var_tmp_")
}
