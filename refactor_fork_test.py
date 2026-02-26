import re

with open("internal/parser/fork_test.go", "r") as f:
    content = f.read()

# Add helpers at the end of the file or before the first test
helpers = """
import (
\t"testing"
\t"time"

\t"github.com/wesm/agentsview/internal/testjsonl"
)

func parseTestContent(t *testing.T, filename, content string) []SessionResult {
\tt.Helper()
\tpath := createTestFile(t, filename, content)
\tresults, err := ParseClaudeSession(path, "proj", "local")
\tif err != nil {
\t\tt.Fatalf("ParseClaudeSession: %v", err)
\t}
\treturn results
}

func formatTime(t time.Time) string {
\treturn t.Format("2006-01-02T15:04:05Z")
}
"""

content = re.sub(r'import \(\n\s+"testing"\n\n\s+"github\.com/wesm/agentsview/internal/testjsonl"\n\)', helpers.strip(), content, count=1)

# Replace repeated parse logic
old_parse_pattern = r'\s*path := createTestFile\(t, "([^"]+)", content\)\n\s*results, err := ParseClaudeSession\(path, "proj", "local"\)\n\s*if err != nil {\n\s*t\.Fatalf\("ParseClaudeSession: %v", err\)\n\s*}'
new_parse_pattern = r'\n\tresults := parseTestContent(t, "\1", content)'
content = re.sub(old_parse_pattern, new_parse_pattern, content)

# Replace Format
content = re.sub(r'\.Format\(\n?\s*"2006-01-02T15:04:05Z",\n?\s*\)', r'.Format("2006-01-02T15:04:05Z")', content)
content = re.sub(r'sess\.EndedAt\.Format\("2006-01-02T15:04:05Z"\)', 'formatTime(sess.EndedAt)', content)
content = re.sub(r'sess\.StartedAt\.Format\("2006-01-02T15:04:05Z"\)', 'formatTime(sess.StartedAt)', content)
content = re.sub(r'results\[0\]\.Session\.EndedAt\.Format\("2006-01-02T15:04:05Z"\)', 'formatTime(results[0].Session.EndedAt)', content)
content = re.sub(r'results\[1\]\.Session\.EndedAt\.Format\("2006-01-02T15:04:05Z"\)', 'formatTime(results[1].Session.EndedAt)', content)

with open("internal/parser/fork_test.go", "w") as f:
    f.write(content)

