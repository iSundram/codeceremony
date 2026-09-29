package mailer

import (
	"strings"
	"testing"
)

const markupPayload = `<img src=x onerror=alert(1)>`

func TestRenderEscapesTheDisplayNameInTheHTMLPart(t *testing.T) {
	registry := NewRegistry()
	rendered, err := registry.Render("welcome", Context{
		"DisplayName":    markupPayload,
		"AppURL":         "https://app.example.org",
		"PreferencesURL": "https://app.example.org/settings/notifications",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(rendered.HTMLBody, "<img") || strings.Contains(rendered.HTMLBody, "onerror=alert(1)>") {
		t.Fatalf("the html part contains live markup: %s", rendered.HTMLBody)
	}
	if !strings.Contains(rendered.HTMLBody, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("expected the display name to be escaped in the html part: %s", rendered.HTMLBody)
	}
	if !strings.Contains(rendered.TextBody, markupPayload) {
		t.Fatalf("expected the text part to keep the value as literal text: %s", rendered.TextBody)
	}
	if !strings.Contains(rendered.HTMLBody, "https://app.example.org/settings/notifications") {
		t.Fatalf("expected a usable preferences link in the html part: %s", rendered.HTMLBody)
	}
}

func TestHTMLPartEscapesEveryUserSuppliedField(t *testing.T) {
	cases := []struct {
		name     string
		template string
		values   Context
	}{
		{
			name:     "display name",
			template: "welcome",
			values:   Context{"DisplayName": markupPayload},
		},
		{
			name:     "team and inviter names",
			template: "team_invite",
			values:   Context{"DisplayName": "Pia", "InviterName": markupPayload, "TeamName": markupPayload, "HackathonName": "Sample Hack 2026", "Message": "join us"},
		},
		{
			name:     "project title",
			template: "submission_received",
			values:   Context{"DisplayName": "Pia", "HackathonName": markupPayload, "ProjectTitle": markupPayload, "ProjectID": "prj_01", "Version": 2},
		},
		{
			name:     "announcement headline and body",
			template: "hackathon_announcement",
			values:   Context{"DisplayName": "Pia", "HackathonName": "Sample Hack 2026", "HackathonURL": "https://app.example.org/hackathons/sample-hack-2026", "Headline": markupPayload, "Body": markupPayload, "UnsubscribeURL": "https://app.example.org/unsubscribe?token=abc"},
		},
		{
			name:     "results placement",
			template: "results_published",
			values:   Context{"DisplayName": "Pia", "HackathonName": markupPayload, "HackathonSlug": "sample-hack-2026", "Placement": markupPayload},
		},
	}
	registry := NewRegistry()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			values := Context{
				"AppURL":         "https://app.example.org",
				"PreferencesURL": "https://app.example.org/settings/notifications",
			}
			for key, value := range testCase.values {
				values[key] = value
			}
			rendered, err := registry.Render(testCase.template, values)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if strings.Contains(rendered.HTMLBody, "<img") {
				t.Fatalf("the html part contains live markup: %s", rendered.HTMLBody)
			}
			if !strings.Contains(rendered.HTMLBody, "&lt;img src=x onerror=alert(1)&gt;") {
				t.Fatalf("expected an escaped value in the html part: %s", rendered.HTMLBody)
			}
		})
	}
}

func TestHTMLPartNeutralisesUnsafeURLValues(t *testing.T) {
	registry := NewRegistry()
	rendered, err := registry.Render("welcome", Context{
		"DisplayName":    "Pia",
		"AppURL":         "https://app.example.org",
		"PreferencesURL": "javascript:alert(1)",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(rendered.HTMLBody, `href="javascript:`) {
		t.Fatalf("expected an unsafe link target to be dropped: %s", rendered.HTMLBody)
	}
	if !strings.Contains(rendered.HTMLBody, "#ZgotmplZ") {
		t.Fatalf("expected the unsafe link target to be replaced, not rendered: %s", rendered.HTMLBody)
	}
}
