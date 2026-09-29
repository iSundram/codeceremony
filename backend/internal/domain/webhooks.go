package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Webhook struct {
	ID             string     `json:"id"`
	EventID        string     `json:"event_id"`
	URL            string     `json:"url"`
	// Secret keys the delivery signature and is persisted. It used to be
	// `json:"-"`, which meant a restart reloaded it as the empty string and
	// every subsequent delivery was signed with an empty HMAC key: the receiver
	// either rejected everything or, having learned to accept "", accepted
	// payloads anyone could forge. An empty secret is now refused at delivery
	// time as well, so the failure is loud rather than silent.
	Secret         string     `json:"secret,omitempty"`
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

// ValidateWebhookHost refuses a destination the portal must not be made to
// fetch on an organizer's behalf.
//
// The scheme check alone is not a control. An organizer is not a trusted party
// in this system — the threat model says so — and a webhook is the one place
// where the portal performs a network request to a URL a user supplied, with
// the portal's own egress and its signed payload attached. Without this, an
// organizer points one at 169.254.169.254 and reads the instance credentials
// out of the response, or at an internal admin endpoint and gets a signed,
// authenticated POST from inside the network.
//
// The check is by name and by literal range, and it is deliberately not
// sufficient on its own: a public hostname can resolve to a private address.
// The transport therefore also pins the dialer, because validating here and
// resolving there is the gap this comment exists to prevent someone closing
// only the first half of.
func ValidateWebhookHost(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w: webhook url is not a valid url", ErrValidation)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: webhook url must have a host", ErrValidation)
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "localhost" || strings.HasSuffix(name, ".localhost") ||
		strings.HasSuffix(name, ".local") || name == "metadata.google.internal" {
		return fmt.Errorf("%w: webhook url must not target a local or metadata host", ErrValidation)
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateOrMetadataIP(ip) {
			return fmt.Errorf("%w: webhook url must not target a private, loopback or link-local address", ErrValidation)
		}
		return nil
	}
	return nil
}

// IsPrivateOrMetadataIP reports whether an address is one the portal must never
// dial on a user's behalf. Loopback, RFC1918, link-local (which is where the
// cloud metadata service lives), unique-local, and the unspecified address.
func IsPrivateOrMetadataIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	// net.IP.IsPrivate covers RFC1918 and fc00::/7. The ranges below cover what
	// it does not: the carrier-grade NAT block, the benchmarking block, and
	// 100.64/10 which is routinely used inside container networks.
	for _, block := range []*net.IPNet{
		{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},
		{IP: net.IPv4(192, 0, 0, 0), Mask: net.CIDRMask(24, 32)},
		{IP: net.IPv4(198, 18, 0, 0), Mask: net.CIDRMask(15, 32)},
		{IP: net.IPv4(224, 0, 0, 0), Mask: net.CIDRMask(4, 32)},
		{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},
	} {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

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
	if err := ValidateWebhookHost(trimmed); err != nil {
		return err
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
