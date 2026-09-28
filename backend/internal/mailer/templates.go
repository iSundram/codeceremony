package mailer

import (
	"bytes"
	"fmt"
	"html"
	"strings"
	"text/template"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type Template struct {
	Name        string
	Kind        domain.MailKind
	Topic       domain.MailTopic
	Subject     string
	Text        string
	HTML        string
	Description string
}

type Context map[string]any

type Rendered struct {
	Subject  string
	TextBody string
	HTMLBody string
}

type Registry struct {
	templates map[string]Template
}

func NewRegistry() *Registry {
	registry := &Registry{templates: map[string]Template{}}
	for _, tmpl := range defaultTemplates() {
		registry.templates[tmpl.Name] = tmpl
	}
	return registry
}

func (r *Registry) Get(name string) (Template, bool) {
	tmpl, ok := r.templates[name]
	return tmpl, ok
}

func (r *Registry) List() []Template {
	result := make([]Template, 0, len(r.templates))
	for _, tmpl := range r.templates {
		result = append(result, tmpl)
	}
	return result
}

func (r *Registry) Render(name string, values Context) (Rendered, error) {
	tmpl, ok := r.templates[name]
	if !ok {
		return Rendered{}, fmt.Errorf("unknown mail template %q", name)
	}
	subject, err := renderString(tmpl.Subject, values)
	if err != nil {
		return Rendered{}, err
	}
	textBody, err := renderString(tmpl.Text, values)
	if err != nil {
		return Rendered{}, err
	}
	htmlBody := tmpl.HTML
	if htmlBody == "" {
		htmlBody = layoutTemplate
	}
	htmlValues := map[string]any{"Subject": subject, "Body": textBody, "Paragraphs": paragraphs(textBody)}
	if _, ok := htmlValues["PreferencesURL"]; !ok {
		htmlValues["PreferencesURL"] = ""
	}
	for key, value := range values {
		htmlValues[key] = value
	}
	htmlBody, err = renderString(htmlBody, htmlValues)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Subject: subject, TextBody: textBody, HTMLBody: htmlBody}, nil
}

func paragraphs(body string) []string {
	parts := strings.Split(body, "\n\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func renderString(source string, values Context) (string, error) {
	parsed, err := template.New("mail").Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	buffer := &bytes.Buffer{}
	if err := parsed.Execute(buffer, values); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

const layoutTemplate = `<!doctype html>
<html lang="en">
<body style="margin:0;padding:24px;background:#F4F3F1;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#33343B;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border:1px solid #E6E4E0;border-radius:8px;">
<tr><td style="padding:24px 32px;border-bottom:1px solid #E6E4E0;">
<span style="font-size:16px;font-weight:600;color:#48547C;">CodeCeremony</span>
</td></tr>
<tr><td style="padding:32px;font-size:15px;line-height:24px;">
<h1 style="margin:0 0 16px;font-size:20px;color:#33343B;">{{ .Subject }}</h1>
{{ range $line := .Paragraphs }}<p style="margin:0 0 16px;">{{ $line }}</p>
{{ end }}
</td></tr>
<tr><td style="padding:16px 32px;border-top:1px solid #E6E4E0;font-size:12px;color:#77736E;">
{{ if .PreferencesURL }}You are receiving this message from CodeCeremony. Manage your email preferences at <a href="{{ .PreferencesURL }}" style="color:#749DD0;">{{ .PreferencesURL }}</a>.{{ else }}You are receiving this message from CodeCeremony.{{ end }}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

func defaultTemplates() []Template {
	return []Template{
		{
			Name:        "welcome",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicAccountLifecycle,
			Subject:     "Welcome to CodeCeremony",
			Text:        "Hi {{ .DisplayName }},\n\nYour CodeCeremony account is ready. Join a hackathon, find a team, or host your own event.\n\nOpen your dashboard: {{ .AppURL }}\n\nManage email preferences: {{ .PreferencesURL }}",
			Description: "Sent once after a participant completes registration.",
		},
		{
			Name:        "verify_email",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicAccountSecurity,
			Subject:     "Confirm your CodeCeremony email address",
			Text:        "Hi {{ .DisplayName }},\n\nConfirm this address so we can send you hackathon updates and judging notices.\n\nConfirm: {{ .VerifyURL }}\n\nIf you did not create this account you can ignore this message.",
			Description: "Email verification with a single-use link.",
		},
		{
			Name:        "password_reset",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicAccountSecurity,
			Subject:     "Reset your CodeCeremony password",
			Text:        "Hi {{ .DisplayName }},\n\nUse this link to choose a new password. It expires in {{ .ResetHours }} hours.\n\nReset: {{ .ResetURL }}\n\nIf you did not request this, no action is needed.",
			Description: "Password reset link.",
		},
		{
			Name:        "account_deletion_scheduled",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicAccountLifecycle,
			Subject:     "Your CodeCeremony account is scheduled for deletion",
			Text:        "Hi {{ .DisplayName }},\n\nYour account and personal data will be deleted on {{ .ScheduledFor }}. Export your data first if you want a copy.\n\nCancel the deletion: {{ .CancelURL }}",
			Description: "Account deletion notice with a cancellation link.",
		},
		{
			Name:        "team_invite",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicTeamActivity,
			Subject:     "{{ .InviterName }} invited you to join {{ .TeamName }}",
			Text:        "Hi {{ .DisplayName }},\n\n{{ .InviterName }} invited you to {{ .TeamName }}{{ if .HackathonName }} on {{ .HackathonName }}{{ end }}.\n\n{{ if .Message }}{{ .Message }}\n\n{{ end }}Review the invite: {{ .AppURL }}/invites",
			Description: "Team invitation.",
		},
		{
			Name:        "submission_received",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicEventActivity,
			Subject:     "We received your submission for {{ .HackathonName }}",
			Text:        "Hi {{ .DisplayName }},\n\n{{ .ProjectTitle }} is recorded as version {{ .Version }}. Edit it any time before the deadline at {{ .AppURL }}/submissions/{{ .ProjectID }}.",
			Description: "Confirms a project submission and revision.",
		},
		{
			Name:        "review_reminder",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicJudging,
			Subject:     "{{ .PendingCount }} projects still need your review",
			Text:        "Hi {{ .DisplayName }},\n\nYou have {{ .PendingCount }} assigned projects left to review for {{ .HackathonName }}. Judging closes {{ .JudgingClose }}.\n\nStart reviewing: {{ .AppURL }}/judge/assignments",
			Description: "Reminds a judge about outstanding assignments.",
		},
		{
			Name:        "results_published",
			Kind:        domain.MailKindTransactional,
			Topic:       domain.MailTopicResults,
			Subject:     "Results are published for {{ .HackathonName }}",
			Text:        "Hi {{ .DisplayName }},\n\n{{ if .Placement }}Your team placed #{{ .Placement }}.\n\n{{ end }}See the leaderboard: {{ .AppURL }}/hackathons/{{ .HackathonSlug }}/leaderboard",
			Description: "Results publication notice with placement.",
		},
		{
			Name:        "hackathon_announcement",
			Kind:        domain.MailKindPromotional,
			Topic:       domain.MailTopicMarketing,
			Subject:     "{{ .HackathonName }}: {{ .Headline }}",
			Text:        "Hi {{ .DisplayName }},\n\n{{ .Headline }}\n\n{{ .Body }}\n\nRegister: {{ .HackathonURL }}\n\nYou are receiving this because you registered on CodeCeremony. Unsubscribe: {{ .UnsubscribeURL }}",
			Description: "Organizer announcement to registered participants.",
		},
		{
			Name:        "weekly_digest",
			Kind:        domain.MailKindPromotional,
			Topic:       domain.MailTopicMarketing,
			Subject:     "Your CodeCeremony week in review",
			Text:        "Hi {{ .DisplayName }},\n\nHackathons: {{ .HackathonCount }}\nInvites waiting: {{ .PendingInvites }}\nAssignments pending: {{ .PendingAssignments }}\n\nOpen CodeCeremony: {{ .AppURL }}\n\nUnsubscribe from the digest: {{ .UnsubscribeURL }}",
			Description: "Optional weekly summary email.",
		},
	}
}

type AnnouncementRequest struct {
	Headline      string
	Body          string
	SendTo        string
	OnlySeeking   bool
	MaxRecipients int
}

func NormalizeAnnouncement(request AnnouncementRequest, now time.Time) (string, string, string) {
	headline := strings.TrimSpace(request.Headline)
	body := strings.TrimSpace(request.Body)
	sendTo := strings.ToLower(strings.TrimSpace(request.SendTo))
	switch sendTo {
	case "", "all":
		return headline, body, "all"
	case "seeking":
		return headline, body, "seeking"
	case "team_captains":
		return headline, body, "team_captains"
	case "judges":
		return headline, body, "judges"
	default:
		return headline, body, sendTo
	}
}

func Escape(value string) string {
	return html.EscapeString(value)
}
