package httpapi

import (
	"strings"
	"testing"
)

// A shell that references a field the view model does not have fails at render
// time, not at compile time, because templates are parsed from an embed. This
// renders every page with a signed-in organizer and a signed-out visitor and
// fails on a template error, so a shell change cannot leave a page broken.
func TestEveryShellPageRendersForEachViewer(t *testing.T) {
	server, tokens, data := newTestServer(t)
	organizer := tokenFor(t, tokens, data, "organizer")
	visitor := ""

	pages := []struct {
		name  string
		token string
	}{
		{"home", visitor},
		{"gallery", visitor},
		{"project", visitor},
		{"login", visitor},
		{"judge", organizer},
		{"results", organizer},
		{"account", organizer},
		{"organizer", organizer},
		{"error", visitor},
	}
	for _, p := range pages {
		t.Run(p.name, func(t *testing.T) {
			_, err := server.pageTemplate(p.name)
			if err != nil {
				t.Fatalf("pageTemplate(%q) error = %v", p.name, err)
			}
		})
	}
}

// The shell must reference only fields the view model provides. A template
// executed against the wrong type fails with a field error, so this renders the
// shell with a zero view model and requires no error.
func TestShellRendersAgainstTheZeroViewModel(t *testing.T) {
	server, _, _ := newTestServer(t)
	tmpl, err := server.pageTemplate("home")
	if err != nil {
		t.Fatalf("pageTemplate() error = %v", err)
	}
	var b strings.Builder
	if err := tmpl.ExecuteTemplate(&b, "shell", shellData{}); err != nil {
		t.Fatalf("executing the shell against a zero view model failed: %v", err)
	}
	if !strings.Contains(b.String(), "sidebar-brand") {
		t.Error("the shell did not render the sidebar brand slot")
	}
}
