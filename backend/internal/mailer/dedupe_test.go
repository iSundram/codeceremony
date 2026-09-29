package mailer

import (
	"log/slog"
	"testing"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

// newTestServiceAtClock builds a service whose clock is under the test's
// control, because the derived dedupe key is a function of the current window.
func newTestServiceAtClock(t *testing.T, now *time.Time, options Options) (*store.Store, *Service) {
	t.Helper()
	hash, err := auth.HashPassword("mailer-dedupe-password")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	data := store.New(seed.Default(hash))
	registry := NewRegistry()
	dispatcher := NewDispatcher(data, registry, &recordingSender{}, DispatcherOptions{
		MaxAttempts: 3,
		BaseBackoff: time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(discard{}, nil)),
	})
	if options.Clock == nil {
		options.Clock = func() time.Time { return *now }
	}
	options.AppURL = "https://app.example.org"
	options.Logger = slog.New(slog.NewTextHandler(discard{}, nil))
	return data, NewService(data, dispatcher, registry, options)
}

func TestDerivedDedupeKeyIsScopedToTheSendWindow(t *testing.T) {
	cases := []struct {
		name     string
		first    time.Time
		second   time.Time
		window   time.Duration
		wantSame bool
	}{
		{
			name:     "same day, hours apart",
			first:    time.Date(2026, time.March, 2, 8, 0, 0, 0, time.UTC),
			second:   time.Date(2026, time.March, 2, 21, 30, 0, 0, time.UTC),
			wantSame: true,
		},
		{
			name:     "next day",
			first:    time.Date(2026, time.March, 2, 21, 30, 0, 0, time.UTC),
			second:   time.Date(2026, time.March, 3, 8, 0, 0, 0, time.UTC),
			wantSame: false,
		},
		{
			name:     "month boundary",
			first:    time.Date(2026, time.March, 31, 23, 0, 0, 0, time.UTC),
			second:   time.Date(2026, time.April, 1, 1, 0, 0, 0, time.UTC),
			wantSame: false,
		},
		{
			name:     "year boundary",
			first:    time.Date(2026, time.December, 31, 23, 0, 0, 0, time.UTC),
			second:   time.Date(2027, time.January, 1, 1, 0, 0, 0, time.UTC),
			wantSame: false,
		},
		{
			name:     "hourly window separates the same day",
			first:    time.Date(2026, time.March, 2, 8, 10, 0, 0, time.UTC),
			second:   time.Date(2026, time.March, 2, 9, 40, 0, 0, time.UTC),
			window:   time.Hour,
			wantSame: false,
		},
		{
			name:     "hourly window collapses the same hour",
			first:    time.Date(2026, time.March, 2, 8, 10, 0, 0, time.UTC),
			second:   time.Date(2026, time.March, 2, 8, 55, 0, 0, time.UTC),
			window:   time.Hour,
			wantSame: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			now := testCase.first
			_, service := newTestServiceAtClock(t, &now, Options{DedupeWindow: testCase.window})
			first := service.deriveDedupeKey("welcome", "participant")

			now = testCase.second
			second := service.deriveDedupeKey("welcome", "participant")

			if (first == second) != testCase.wantSame {
				t.Fatalf("keys %q and %q, want same = %v", first, second, testCase.wantSame)
			}
			if first != service.deriveDedupeKey("welcome", "participant") && testCase.wantSame {
				t.Fatalf("expected a stable key within one window")
			}
		})
	}
}

func TestDerivedDedupeKeySeparatesTemplatesAndRecipients(t *testing.T) {
	now := time.Date(2026, time.March, 2, 8, 0, 0, 0, time.UTC)
	_, service := newTestServiceAtClock(t, &now, Options{})
	keys := map[string]bool{
		service.deriveDedupeKey("welcome", "participant"):       true,
		service.deriveDedupeKey("verify_email", "participant"):  true,
		service.deriveDedupeKey("welcome", "participant_other"): true,
	}
	if len(keys) != 3 {
		t.Fatalf("expected the template and the recipient to both be part of the key: %#v", keys)
	}
}

func TestSendToDedupesWithinTheWindowAndSendsAgainOnALaterDay(t *testing.T) {
	now := time.Date(2026, time.March, 2, 8, 0, 0, 0, time.UTC)
	_, service := newTestServiceAtClock(t, &now, Options{})
	first, err := service.SendTo("participant", "welcome", nil, "")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if first.Status != "queued" {
		t.Fatalf("status = %q, want queued", first.Status)
	}

	now = now.Add(4 * time.Hour)
	sameWindow, err := service.SendTo("participant", "welcome", nil, "")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if sameWindow.ID != first.ID {
		t.Fatalf("expected the second send in the same window to be deduped: %s != %s", sameWindow.ID, first.ID)
	}

	now = now.Add(24 * time.Hour)
	nextDay, err := service.SendTo("participant", "welcome", nil, "")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if nextDay.ID == first.ID {
		t.Fatalf("expected a later day to queue a new message, got the original %s", nextDay.ID)
	}
	if nextDay.Status != "queued" {
		t.Fatalf("status = %q, want queued rather than a silent replay of the sent message", nextDay.Status)
	}
}

func TestExplicitDedupeKeyIgnoresTheSendWindow(t *testing.T) {
	now := time.Date(2026, time.March, 2, 8, 0, 0, 0, time.UTC)
	_, service := newTestServiceAtClock(t, &now, Options{})
	first, err := service.SendTo("participant", "welcome", nil, "caller:chosen-key")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if first.DedupeKey != "caller:chosen-key" {
		t.Fatalf("dedupe key = %q, want the caller's key untouched", first.DedupeKey)
	}
	now = now.Add(72 * time.Hour)
	second, err := service.SendTo("participant", "welcome", nil, "caller:chosen-key")
	if err != nil {
		t.Fatalf("SendTo() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("an explicit key must keep collapsing duplicates: %s != %s", second.ID, first.ID)
	}
}
