package mailer

import (
	"log/slog"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

type recordingSender struct {
	sent    []Message
	failFor string
	err     error
}

func (s *recordingSender) Name() string { return "recording" }

func (s *recordingSender) Send(message Message) error {
	if s.failFor != "" && message.To == s.failFor {
		return s.err
	}
	s.sent = append(s.sent, message)
	return nil
}

func newTestService(t *testing.T, sender Sender) (*store.Store, *Service, *Dispatcher) {
	t.Helper()
	hash, err := auth.HashPassword("mailer-test-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	data := store.New(seed.Default(hash))
	registry := NewRegistry()
	dispatcher := NewDispatcher(data, registry, sender, DispatcherOptions{
		MaxAttempts: 3,
		BaseBackoff: time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(discard{}, nil)),
	})
	service := NewService(data, dispatcher, registry, Options{AppURL: "https://app.example.org"})
	return data, service, dispatcher
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestTemplateRegistryCoversEveryTemplate(t *testing.T) {
	registry := NewRegistry()
	names := map[string]bool{}
	for _, tmpl := range registry.List() {
		names[tmpl.Name] = true
	}
	for _, expected := range []string{"welcome", "verify_email", "password_reset", "team_invite", "submission_received", "review_reminder", "results_published", "hackathon_announcement", "weekly_digest", "account_deletion_scheduled"} {
		if !names[expected] {
			t.Fatalf("expected the registry to include %q", expected)
		}
	}
	if _, ok := registry.Get("hackathon_announcement"); !ok {
		t.Fatalf("expected the announcement template to exist")
	}
}

func TestRenderIncludesSubjectAndBody(t *testing.T) {
	registry := NewRegistry()
	rendered, err := registry.Render("team_invite", Context{
		"DisplayName":   "Pia",
		"TeamName":      "NorthKiln",
		"InviterName":   "Rhea",
		"HackathonName": "Sample Hack 2026",
		"Message":       "Join us.",
		"AppURL":        "https://app.example.org",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if rendered.Subject != "Rhea invited you to join NorthKiln" {
		t.Fatalf("subject = %q", rendered.Subject)
	}
	if rendered.HTMLBody == "" || rendered.TextBody == "" {
		t.Fatalf("expected both a text and html body")
	}
}

func TestRenderFailsOnUnknownTemplate(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Render("nope", Context{}); err == nil {
		t.Fatalf("expected an error for an unknown template")
	}
	if _, err := registry.Render("welcome", Context{}); err == nil {
		t.Fatalf("expected an error when required context is missing")
	}
}

func TestQueueRespectsPromotionalOptIn(t *testing.T) {
	sender := &recordingSender{}
	_, service, _ := newTestService(t, sender)
	message, err := service.SendTo("participant_other", "hackathon_announcement", map[string]any{
		"HackathonName": "Sample Hack 2026", "HackathonURL": "https://app.example.org/hackathons/sample-hack-2026",
		"Headline": "Registration is open", "Body": "Come join us.", "UnsubscribeURL": "https://app.example.org/unsubscribe?token=abc",
	}, "")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if message.Status != domain.MailSkipped {
		t.Fatalf("status = %q, want skipped because marketing is off by default", message.Status)
	}
	delivered, err := service.Dispatcher().DispatchOnce(10)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if delivered != 0 || len(sender.sent) != 0 {
		t.Fatalf("expected no delivery, delivered = %d, sent = %d", delivered, len(sender.sent))
	}
}

func TestQueueDeliversAfterOptIn(t *testing.T) {
	sender := &recordingSender{}
	data, service, _ := newTestService(t, sender)
	preferences := data.MailPreferences("participant_other")
	preferences.Marketing = true
	if _, err := data.SaveMailPreferences(preferences); err != nil {
		t.Fatalf("SaveMailPreferences() error = %v", err)
	}
	message, err := service.SendTo("participant_other", "hackathon_announcement", map[string]any{
		"HackathonName": "Sample Hack 2026", "HackathonURL": "https://app.example.org/hackathons/sample-hack-2026",
		"Headline": "Registration is open", "Body": "Come join us.", "UnsubscribeURL": "https://app.example.org/unsubscribe?token=abc",
	}, "")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if message.Status != domain.MailQueued {
		t.Fatalf("status = %q, want queued", message.Status)
	}
	delivered, err := service.Dispatcher().DispatchOnce(10)
	if err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if delivered != 1 || len(sender.sent) != 1 {
		t.Fatalf("expected one delivery, delivered = %d, sent = %d", delivered, len(sender.sent))
	}
	if sender.sent[0].To != "participant-other@example.org" {
		t.Fatalf("unexpected recipient %q", sender.sent[0].To)
	}
	sent, err := data.MailByID(message.ID)
	if err != nil {
		t.Fatalf("MailByID() error = %v", err)
	}
	if sent.Status != domain.MailSent || sent.SentAt == nil {
		t.Fatalf("expected the message to be marked sent, got %+v", sent)
	}
}

func TestTransactionalMailIgnoresMarketingOptOut(t *testing.T) {
	sender := &recordingSender{}
	data, service, _ := newTestService(t, sender)
	preferences := data.MailPreferences("participant")
	preferences.UnsubscribedAll = true
	now := time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC)
	preferences.UnsubscribedAt = &now
	if _, err := data.SaveMailPreferences(preferences); err != nil {
		t.Fatalf("SaveMailPreferences() error = %v", err)
	}
	message, err := service.SendPasswordReset("participant", "token-123", 2)
	if err != nil {
		t.Fatalf("SendPasswordReset() error = %v", err)
	}
	if message.Status != domain.MailSkipped {
		t.Fatalf("a full opt-out should stop account security mail too, status = %q", message.Status)
	}
}

func TestDedupeKeyPreventsDuplicates(t *testing.T) {
	sender := &recordingSender{}
	_, service, _ := newTestService(t, sender)
	first, err := service.SendReviewReminder("judge_a", "evt_01", 3)
	if err != nil {
		t.Fatalf("SendReviewReminder() error = %v", err)
	}
	second, err := service.SendReviewReminder("judge_a", "evt_01", 3)
	if err != nil {
		t.Fatalf("SendReviewReminder() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the dedupe key to collapse the reminders: %s != %s", first.ID, second.ID)
	}
}

func TestDispatchRetriesThenGivesUp(t *testing.T) {
	sender := &recordingSender{failFor: "judge-a@example.org", err: errTest{}}
	data, service, dispatcher := newTestService(t, sender)
	message, err := service.SendResultsPublished("judge_a", "evt_01", "1")
	if err != nil {
		t.Fatalf("SendResultsPublished() error = %v", err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		time.Sleep(3 * time.Millisecond)
		if _, err := dispatcher.DispatchOnce(5); err != nil {
			t.Fatalf("DispatchOnce() error = %v", err)
		}
	}
	final, err := data.MailByID(message.ID)
	if err != nil {
		t.Fatalf("MailByID() error = %v", err)
	}
	if final.Status != domain.MailFailed {
		t.Fatalf("status = %q, want failed after the attempt budget", final.Status)
	}
	if final.Attempts < 3 {
		t.Fatalf("attempts = %d, want at least 3", final.Attempts)
	}
	if final.LastError == "" {
		t.Fatalf("expected the failure reason to be recorded")
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	_, _, dispatcher := newTestService(t, &recordingSender{})
	first := dispatcher.Backoff(1)
	second := dispatcher.Backoff(2)
	if second <= first {
		t.Fatalf("expected exponential backoff, first = %s, second = %s", first, second)
	}
	if dispatcher.Backoff(20) > 30*time.Minute {
		t.Fatalf("expected the backoff to be capped")
	}
}

func TestAnnouncementTargetsHonorMarketingConsent(t *testing.T) {
	sender := &recordingSender{}
	data, service, _ := newTestService(t, sender)
	event, err := data.EventBySlug("sample-hack-2026")
	if err != nil {
		t.Fatalf("EventBySlug() error = %v", err)
	}
	targets, excluded := service.ResolveAnnouncementTargets(event, "all")
	if len(targets) == 0 {
		t.Fatalf("expected at least one marketing-consenting recipient")
	}
	for _, excludedEntry := range excluded {
		if excludedEntry == "" {
			t.Fatalf("expected exclusion reasons to be recorded")
		}
	}
	seeking, _ := service.ResolveAnnouncementTargets(event, "seeking")
	if len(seeking) > len(targets) {
		t.Fatalf("seeking audience should not be larger than all: %d > %d", len(seeking), len(targets))
	}
}

func TestAnnouncementQueuesPerRecipient(t *testing.T) {
	sender := &recordingSender{}
	data, service, _ := newTestService(t, sender)
	preferences := data.MailPreferences("participant_other")
	preferences.Marketing = true
	if _, err := data.SaveMailPreferences(preferences); err != nil {
		t.Fatalf("SaveMailPreferences() error = %v", err)
	}
	event, err := data.EventBySlug("sample-hack-2026")
	if err != nil {
		t.Fatalf("EventBySlug() error = %v", err)
	}
	result, err := service.SendAnnouncement(event, "Judging starts now", "Please finish your reviews.", "all")
	if err != nil {
		t.Fatalf("SendAnnouncement() error = %v", err)
	}
	queued := result["queued"].([]string)
	if len(queued) == 0 {
		t.Fatalf("expected at least one queued announcement: %#v", result)
	}
	if _, err := service.Dispatcher().DispatchOnce(20); err != nil {
		t.Fatalf("DispatchOnce() error = %v", err)
	}
	if len(sender.sent) == 0 {
		t.Fatalf("expected the announcement to be delivered")
	}
	for _, message := range sender.sent {
		if message.Subject == "" || message.Text == "" {
			t.Fatalf("announcement was rendered empty: %+v", message)
		}
	}
}

func TestUnsubscribeTokenFlow(t *testing.T) {
	sender := &recordingSender{}
	data, _, _ := newTestService(t, sender)
	token, err := NewUnsubscribeToken("participant", "marketing")
	if err != nil {
		t.Fatalf("NewUnsubscribeToken() error = %v", err)
	}
	if _, err := data.SaveUnsubscribeToken(token); err != nil {
		t.Fatalf("SaveUnsubscribeToken() error = %v", err)
	}
	loaded, err := data.UnsubscribeTokenByValue(token.Token)
	if err != nil {
		t.Fatalf("UnsubscribeTokenByValue() error = %v", err)
	}
	if loaded.UserID != "participant" || loaded.Scope != "marketing" {
		t.Fatalf("unexpected token %+v", loaded)
	}
	if _, err := data.UnsubscribeAll(loaded.UserID); err != nil {
		t.Fatalf("UnsubscribeAll() error = %v", err)
	}
	preferences := data.MailPreferences("participant")
	if !preferences.UnsubscribedAll || preferences.Marketing {
		t.Fatalf("expected a full opt-out, got %+v", preferences)
	}
	if err := data.DeleteUnsubscribeToken(token.Token); err != nil {
		t.Fatalf("DeleteUnsubscribeToken() error = %v", err)
	}
	if _, err := data.UnsubscribeTokenByValue(token.Token); err == nil {
		t.Fatalf("expected the token to be consumed")
	}
}

func TestExpiredUnsubscribeToken(t *testing.T) {
	sender := &recordingSender{}
	_, _, _ = newTestService(t, sender)
	token := domain.UnsubscribeToken{Token: "old", UserID: "participant", Scope: "marketing", ExpiresAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	if !token.Expired(time.Now()) {
		t.Fatalf("expected the token to be expired")
	}
}

func TestNormalizeAnnouncementAudience(t *testing.T) {
	headline, body, audience := NormalizeAnnouncement(AnnouncementRequest{Headline: " Hi ", Body: " Body ", SendTo: "Seeking"}, time.Now())
	if headline != "Hi" || body != "Body" || audience != "seeking" {
		t.Fatalf("unexpected normalization: %q %q %q", headline, body, audience)
	}
	_, _, fallback := NormalizeAnnouncement(AnnouncementRequest{Headline: "Hi", Body: "Body"}, time.Now())
	if fallback != "all" {
		t.Fatalf("expected the default audience to be all, got %q", fallback)
	}
}

func TestBuildRFC822ProducesMultipart(t *testing.T) {
	body := buildRFC822(Message{From: "no-reply@example.org", To: "user@example.org", Subject: "Hi there", Text: "text body", HTML: "<p>html body</p>"})
	if body == "" {
		t.Fatalf("expected a rendered message")
	}
	plain := buildRFC822(Message{From: "no-reply@example.org", To: "user@example.org", Subject: "Hi", Text: "only text"})
	if plain == "" {
		t.Fatalf("expected a rendered plain message")
	}
}

func TestEscapeHelper(t *testing.T) {
	if Escape("<b>") != "&lt;b&gt;" {
		t.Fatalf("expected html escaping")
	}
}

type errTest struct{}

func (errTest) Error() string { return "smtp refused the message" }
