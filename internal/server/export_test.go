package server

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wesm/agentsview/internal/db"
	"github.com/wesm/agentsview/internal/dbtest"
)

// testSession returns a *db.Session with sensible defaults.
// Override fields after calling or via functional options.
func testSession(
	opts ...func(*db.Session),
) *db.Session {
	s := &db.Session{
		ID:           "test-id",
		Project:      "proj",
		Agent:        "claude",
		MessageCount: 0,
		StartedAt:    dbtest.Ptr("2025-01-15T10:00:00Z"),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// stubServer returns an httptest.Server that responds with
// the given status code and body. Caller must defer ts.Close().
func stubServer(
	status int, body string,
) *httptest.Server {
	return httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				if body != "" {
					w.Write([]byte(body))
				}
			},
		),
	)
}

// assertContextCancelled checks that err is non-nil and
// wraps context.Canceled.
func assertContextCancelled(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) &&
		!strings.Contains(
			err.Error(), "context canceled",
		) {
		t.Errorf(
			"expected context.Canceled, got: %v", err,
		)
	}
}

func TestFormatTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"RFC3339",
			"2025-01-15T10:30:00Z",
			"2025-01-15 10:30:00",
		},
		{
			"RFC3339Nano",
			"2025-06-01T08:15:30.123456789Z",
			"2025-06-01 08:15:30",
		},
		{
			"RFC3339_WithOffset",
			"2025-03-20T14:00:00+05:00",
			"2025-03-20 14:00:00",
		},
		{
			"Empty",
			"",
			"",
		},
		{
			"Unparseable_ReturnsRaw",
			"not-a-timestamp",
			"not-a-timestamp",
		},
		{
			"Midnight",
			"2025-12-31T00:00:00Z",
			"2025-12-31 00:00:00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatTimestamp(tt.in)
			if got != tt.want {
				t.Errorf(
					"formatTimestamp(%q) = %q, want %q",
					tt.in, got, tt.want,
				)
			}
		})
	}
}

func TestFormatDateShort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *string
		want string
	}{
		{"Nil", nil, "unknown"},
		{"Empty", dbtest.Ptr(""), "unknown"},
		{
			"Valid",
			dbtest.Ptr("2025-01-15T10:30:00Z"),
			"20250115",
		},
		{
			"Nano",
			dbtest.Ptr("2025-06-01T08:15:30.999Z"),
			"20250601",
		},
		{
			"Unparseable",
			dbtest.Ptr("garbage"),
			"unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatDateShort(tt.in)
			if got != tt.want {
				t.Errorf(
					"formatDateShort(%v) = %q, want %q",
					tt.in, got, tt.want,
				)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		in    string
		valid bool
	}{
		{"RFC3339", "2025-01-15T10:30:00Z", true},
		{"RFC3339Nano", "2025-01-15T10:30:00.123Z", true},
		{"WithOffset", "2025-01-15T10:30:00+02:00", true},
		{"Invalid", "January 15th", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, ok := parseTimestamp(tt.in)
			if ok != tt.valid {
				t.Errorf(
					"parseTimestamp(%q) ok=%v, want %v",
					tt.in, ok, tt.valid,
				)
			}
		})
	}
}

func TestFormatContentForExport_Escaping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			"HTMLEntitiesEscaped",
			`<script>alert("xss")</script>`,
			[]string{
				"&lt;script&gt;",
				"&lt;/script&gt;",
			},
			[]string{"<script>"},
		},
		{
			"AmpersandEscaped",
			"foo & bar < baz",
			[]string{"foo &amp; bar &lt; baz"},
			[]string{"foo & bar"},
		},
		{
			"CodeBlock",
			"```go\nfmt.Println(\"hello\")\n```",
			[]string{
				"<pre><code>",
				"</code></pre>",
			},
			nil,
		},
		{
			"InlineCode",
			"use `fmt.Println` here",
			[]string{"<code>fmt.Println</code>"},
			nil,
		},
		{
			"ThinkingBlock",
			"[Thinking]\nI need to consider this",
			[]string{
				`class="thinking-block"`,
				`class="thinking-label"`,
			},
			nil,
		},
		{
			"ToolBlock",
			"[Read file.go]\ncontent here",
			[]string{`class="tool-block"`},
			nil,
		},
		{
			"BashToolBlock",
			"[Bash ls -la]\noutput",
			[]string{`class="tool-block"`},
			nil,
		},
		{
			"EmptyInput",
			"",
			[]string{""},
			nil,
		},
		{
			"NestedHTMLInCode",
			"```\n<div>not rendered</div>\n```",
			[]string{
				"&lt;div&gt;not rendered&lt;/div&gt;",
			},
			[]string{"<div>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatContentForExport(tt.input)
			assertContainsAll(t, got, tt.contains)
			assertContainsNone(t, got, tt.excludes)
		})
	}
}

func TestIsThinkingOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			"PureThinking",
			"[Thinking]\nDeep thoughts here",
			true,
		},
		{
			"ThinkingThenToolBlock",
			"[Thinking]\nthoughts\n[Read file.go]\ncontent",
			false,
		},
		{
			"NoThinking",
			"Just regular text",
			false,
		},
		{
			"Empty",
			"",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isThinkingOnly(tt.in)
			if got != tt.want {
				t.Errorf(
					"isThinkingOnly(%q) = %v, want %v",
					tt.in, got, tt.want,
				)
			}
		})
	}
}

func TestGenerateExportHTML_Structure(t *testing.T) {
	t.Parallel()
	session := testSession(func(s *db.Session) {
		s.Project = "my-project"
		s.MessageCount = 2
		s.FirstMessage = dbtest.Ptr("Hello")
	})
	msgs := []db.Message{
		{
			SessionID: "test-id", Ordinal: 0,
			Role: "user", Content: "Hello agent",
			Timestamp: "2025-01-15T10:00:00Z",
		},
		{
			SessionID: "test-id", Ordinal: 1,
			Role:      "assistant",
			Content:   "Hi! How can I help?",
			Timestamp: "2025-01-15T10:00:05Z",
		},
	}

	html := generateExportHTML(session, msgs)

	assertContainsAll(t, html, []string{
		"<!DOCTYPE html>",
		"my-project",
		"Claude",
		"2 messages",
		`class="message user"`,
		`class="message assistant"`,
		"Hello agent",
		"Hi! How can I help?",
		"2025-01-15 10:00:00",
		"2025-01-15 10:00:05",
	})
}

func TestGenerateExportHTML_ThinkingOnlyClass(t *testing.T) {
	t.Parallel()
	session := testSession(func(s *db.Session) {
		s.MessageCount = 1
	})
	msgs := []db.Message{
		{
			SessionID: "test-id", Ordinal: 0,
			Role:      "assistant",
			Content:   "[Thinking]\nJust internal thoughts",
			Timestamp: "2025-01-15T10:00:00Z",
		},
	}

	html := generateExportHTML(session, msgs)
	if !strings.Contains(html, "thinking-only") {
		t.Error("expected thinking-only class for" +
			" thinking-only message")
	}
}

func TestGenerateExportHTML_EscapesHostileInput(t *testing.T) {
	t.Parallel()
	session := testSession(func(s *db.Session) {
		s.Project = `<img src=x onerror=alert(1)>`
		s.MessageCount = 1
	})
	msgs := []db.Message{
		{
			SessionID: "test-id", Ordinal: 0,
			Role:      "user",
			Content:   `<script>alert("xss")</script>`,
			Timestamp: "2025-01-15T10:00:00Z",
		},
	}

	out := generateExportHTML(session, msgs)

	// Template auto-escapes the <img> tag in project name
	if strings.Contains(out, "<img src=x") {
		t.Error("project name XSS: raw <img> tag not escaped")
	}
	// Content is escaped by formatContentForExport
	if strings.Contains(out, "<script>alert") {
		t.Error("message content XSS not escaped")
	}
}

func TestGenerateExportHTML_CodexAgent(t *testing.T) {
	t.Parallel()
	session := testSession(func(s *db.Session) {
		s.Agent = "codex"
	})

	html := generateExportHTML(session, nil)
	if !strings.Contains(html, "Codex") {
		t.Error("expected Codex display name for codex agent")
	}
}

func TestGenerateExportHTML_NilStartedAt(t *testing.T) {
	t.Parallel()
	session := testSession(func(s *db.Session) {
		s.StartedAt = nil
	})

	html := generateExportHTML(session, nil)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected valid HTML even with nil StartedAt")
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"Clean", "foo-bar.html", "foo-bar.html"},
		{"Spaces", "my file.html", "my_file.html"},
		{
			"SpecialChars",
			"a/b:c*d?.html",
			"a_b_c_d_.html",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeFilename(tt.in)
			if got != tt.want {
				t.Errorf(
					"sanitizeFilename(%q) = %q, want %q",
					tt.in, got, tt.want,
				)
			}
		})
	}
}

func TestTruncateStr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"Short", "hi", 10, "hi"},
		{"Exact", "hello", 5, "hello"},
		{"Long", "hello world", 5, "hello..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateStr(tt.in, tt.max)
			if got != tt.want {
				t.Errorf(
					"truncateStr(%q, %d) = %q, want %q",
					tt.in, tt.max, got, tt.want,
				)
			}
		})
	}
}

// TestExportTemplateValid ensures the template parses and
// renders without error for a minimal input.
func TestExportTemplateValid(t *testing.T) {
	t.Parallel()
	data := exportData{
		Project:      "test",
		Agent:        "Claude",
		MessageCount: 1,
		StartedAt:    "2025-01-15 10:00:00",
		Messages: []exportMessage{
			{
				RoleClass:   "user",
				Role:        "user",
				Timestamp:   "2025-01-15 10:00:00",
				ContentHTML: template.HTML("hello"),
			},
		},
	}
	var b strings.Builder
	if err := exportTmpl.Execute(&b, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}
	if !strings.Contains(b.String(), "<!DOCTYPE html>") {
		t.Error("expected valid HTML doctype")
	}
}

// --- GitHub API mock tests ---

func TestCreateGist_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("Authorization") != "token test-tok" {
				t.Error("missing or wrong Authorization header")
			}
			if r.Header.Get("User-Agent") != "agentsview" {
				t.Error("missing User-Agent header")
			}

			w.WriteHeader(http.StatusCreated)
			resp := gistResponse{
				ID:      "abc123",
				HTMLURL: "https://gist.github.com/abc123",
			}
			resp.Owner.Login = "testuser"
			json.NewEncoder(w).Encode(resp)
		}),
	)
	defer ts.Close()

	got, err := createGistWithURL(
		context.Background(),
		ts.URL, "test-tok", "f.html", "desc", "<html>",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "abc123" {
		t.Errorf("ID = %q, want abc123", got.ID)
	}
	if got.HTMLURL != "https://gist.github.com/abc123" {
		t.Errorf("HTMLURL = %q", got.HTMLURL)
	}
	if got.Owner.Login != "testuser" {
		t.Errorf("Owner.Login = %q", got.Owner.Login)
	}
}

func TestCreateGist_APIError(t *testing.T) {
	t.Parallel()
	ts := stubServer(
		http.StatusUnprocessableEntity,
		`{"message":"Validation Failed"}`,
	)
	defer ts.Close()

	_, err := createGistWithURL(
		context.Background(),
		ts.URL, "tok", "f.html", "desc", "content",
	)
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestCreateGist_MalformedJSON(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusOK, "not json")
	defer ts.Close()

	_, err := createGistWithURL(
		context.Background(),
		ts.URL, "tok", "f.html", "desc", "content",
	)
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should mention parsing: %v", err)
	}
}

func TestCreateGist_MissingFields(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusCreated, `{}`)
	defer ts.Close()

	got, err := createGistWithURL(
		context.Background(),
		ts.URL, "tok", "f.html", "desc", "content",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Missing fields decode as zero values
	if got.ID != "" {
		t.Errorf("expected empty ID, got %q", got.ID)
	}
	if got.Owner.Login != "" {
		t.Errorf(
			"expected empty Owner.Login, got %q",
			got.Owner.Login,
		)
	}
}

func TestValidateGithubToken_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.Header.Get("Authorization") != "token good-tok" {
				t.Error("missing or wrong Authorization header")
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(
				map[string]string{"login": "octocat"},
			)
		}),
	)
	defer ts.Close()

	login, err := validateGithubTokenWithURL(context.Background(), ts.URL, "good-tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if login != "octocat" {
		t.Errorf("login = %q, want octocat", login)
	}
}

func TestValidateGithubToken_Unauthorized(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusUnauthorized, "")
	defer ts.Close()

	_, err := validateGithubTokenWithURL(context.Background(), ts.URL, "bad-tok")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid: %v", err)
	}
}

func TestValidateGithubToken_ServerError(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusInternalServerError, "")
	defer ts.Close()

	_, err := validateGithubTokenWithURL(context.Background(), ts.URL, "tok")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestValidateGithubToken_MalformedJSON(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusOK, "{broken")
	defer ts.Close()

	_, err := validateGithubTokenWithURL(context.Background(), ts.URL, "tok")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should mention parsing: %v", err)
	}
}

func TestCreateGist_ContextCancelled(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusOK, "")
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := createGistWithURL(
		ctx,
		ts.URL, "tok", "f.html", "desc", "content",
	)
	assertContextCancelled(t, err)
}

func TestValidateGithubToken_ContextCancelled(t *testing.T) {
	t.Parallel()
	ts := stubServer(http.StatusOK, "")
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := validateGithubTokenWithURL(
		ctx, ts.URL, "tok",
	)
	assertContextCancelled(t, err)
}
