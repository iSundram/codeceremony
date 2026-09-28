package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Webhook struct {
	ID             string     `json:"id"`
	EventID        string     `json:"event_id"`
	URL            string     `json:"url"`
	Secret         string     `json:"-"`
	Events         []string   `json:"events"`
	Active         bool       `json:"active"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
	FailureCount   int        `json:"failure_count"`
	DeliveryCount  int        `json:"delivery_count"`
}

var allowedWebhookEvents = map[string]struct{}{
	"submission.created":     {},
	"submission.revised":     {},
	"submission.submitted":   {},
	"results.published":      {},
	"results.unpublished":    {},
	"vote.closed":            {},
	"team.member_joined":     {},
	"comment.posted":         {},
	"assignment.revoked":     {},
	"hackathon.announcement": {},
}

var allowedWebhookSchemes = []string{"https://"}

func (w Webhook) Validate() error {
	trimmed := strings.TrimSpace(w.URL)
	if trimmed == "" {
		return fmt.Errorf("%w: webhook url is required", ErrValidation)
	}
	allowed := false
	for _, scheme := range allowedWebhookSchemes {
		if strings.HasPrefix(strings.ToLower(trimmed), scheme) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("%w: webhook url must use https", ErrValidation)
	}
	if len(w.Events) == 0 {
		return fmt.Errorf("%w: subscribe to at least one event", ErrValidation)
	}
	for _, event := range w.Events {
		if _, ok := allowedWebhookEvents[event]; !ok {
			return fmt.Errorf("%w: unknown webhook event %q", ErrValidation, event)
		}
	}
	if strings.TrimSpace(w.Secret) == "" {
		return fmt.Errorf("%w: a signing secret is required", ErrValidation)
	}
	if len(w.Secret) < 16 {
		return fmt.Errorf("%w: the signing secret must be at least 16 characters", ErrValidation)
	}
	return nil
}

func (w Webhook) SubscribedTo(event string) bool {
	for _, subscribed := range w.Events {
		if subscribed == event {
			return true
		}
	}
	return false
}

func WebhookEventNames() []string {
	names := make([]string, 0, len(allowedWebhookEvents))
	for name := range allowedWebhookEvents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

type WebhookDelivery struct {
	ID           string         `json:"id"`
	WebhookID    string         `json:"webhook_id"`
	EventID      string         `json:"event_id,omitempty"`
	Event        string         `json:"event"`
	Payload      string         `json:"payload"`
	Status       DeliveryStatus `json:"status"`
	Attempts     int            `json:"attempts"`
	ResponseCode int            `json:"response_code,omitempty"`
	ResponseBody string         `json:"response_body,omitempty"`
	LastError    string         `json:"last_error,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	DeliveredAt  *time.Time     `json:"delivered_at,omitempty"`
	NextAttempt  time.Time      `json:"next_attempt"`
}

type Envelope struct {
	ID        string         `json:"id"`
	Event     string         `json:"event"`
	Hackathon string         `json:"hackathon,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Data      map[string]any `json:"data"`
}

// Sign computes the signature header for a webhook delivery. The signed payload
// is the timestamp, a period, and the body, so replays are detectable.
func Sign(secret string, timestamp time.Time, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp.Unix(), 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "t=" + strconv.FormatInt(timestamp.Unix(), 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(secret string, header string, body []byte, tolerance time.Duration) error {
	parts := strings.Split(header, ",")
	if len(parts) != 2 {
		return fmt.Errorf("malformed signature header")
	}
	var timestamp int64
	var provided string
	for _, part := range parts {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			return fmt.Errorf("malformed signature header")
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("malformed signature timestamp")
			}
			timestamp = parsed
		case "v1":
			provided = value
		default:
			return fmt.Errorf("unknown signature part %q", key)
		}
	}
	if time.Since(time.Unix(timestamp, 0)) > tolerance {
		return fmt.Errorf("signature timestamp is outside the tolerance window")
	}
	expected := Sign(secret, time.Unix(timestamp, 0), body)
	if !hmac.Equal([]byte(expected), []byte("t="+strconv.FormatInt(timestamp, 10)+",v1="+provided)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
