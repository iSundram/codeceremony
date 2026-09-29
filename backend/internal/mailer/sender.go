package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/domain"
)

type Message struct {
	From    string
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender takes a context so a single delivery can be bounded by the dispatcher
// instead of by the slowest relay the network offers.
type Sender interface {
	Send(ctx context.Context, message Message) error
	Name() string
}

type LogSender struct {
	logger *slog.Logger
}

func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{logger: logger}
}

func (s *LogSender) Name() string { return "log" }

func (s *LogSender) Send(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.logger.Info("mail delivered to log sink",
		"to", message.To,
		"subject", message.Subject,
		"body_preview", preview(message.Text),
	)
	return nil
}

type SMTPSender struct {
	cfg     config.Config
	logger  *slog.Logger
	dialTLS func(ctx context.Context, address string, tlsConfig *tls.Config) (net.Conn, error)
}

func NewSMTPSender(cfg config.Config, logger *slog.Logger) *SMTPSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &SMTPSender{cfg: cfg, logger: logger, dialTLS: func(ctx context.Context, address string, tlsConfig *tls.Config) (net.Conn, error) {
		dialer := &tls.Dialer{NetDialer: &net.Dialer{}, Config: tlsConfig}
		return dialer.DialContext(ctx, "tcp", address)
	}}
}

func (s *SMTPSender) Name() string { return "smtp" }

func (s *SMTPSender) Send(ctx context.Context, message Message) error {
	from := s.cfg.SMTPFrom
	if message.From != "" {
		from = message.From
	}
	address := net.JoinHostPort(s.cfg.SMTPHost, strconv.Itoa(s.cfg.SMTPPort))
	timeout := 15 * time.Second
	var conn net.Conn
	var err error
	switch s.cfg.SMTPEncryption {
	case "tls":
		conn, err = s.dialTLS(ctx, address, &tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12})
	default:
		dialer := &net.Dialer{Timeout: timeout}
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadlineFor(ctx, 30*time.Second)); err != nil {
		return fmt.Errorf("smtp deadline: %w", err)
	}
	client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()
	if s.cfg.SMTPEncryption == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: s.cfg.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}
	if s.cfg.SMTPUsername != "" {
		auth := smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := io.WriteString(writer, buildRFC822(message)); err != nil {
		writer.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp body close: %w", err)
	}
	return client.Quit()
}

func buildRFC822(message Message) string {
	headers := []string{
		"From: " + message.From,
		"To: " + message.To,
		"Subject: " + encodeHeader(message.Subject),
		"MIME-Version: 1.0",
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
	}
	if strings.TrimSpace(message.HTML) == "" {
		headers = append(headers, "Content-Type: text/plain; charset=utf-8")
		return strings.Join(headers, "\r\n") + "\r\n\r\n" + message.Text
	}
	boundary := "codeceremony-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	headers = append(headers, `Content-Type: multipart/alternative; boundary="`+boundary+`"`)
	body := strings.Join(headers, "\r\n") + "\r\n\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + message.Text + "\r\n" +
		"--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + message.HTML + "\r\n" +
		"--" + boundary + "--\r\n"
	return body
}

func encodeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if isASCII(value) {
		return value
	}
	return mimeEncoded(value)
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}

func preview(value string) string {
	collapsed := strings.Join(strings.Fields(value), " ")
	if len(collapsed) > 120 {
		return collapsed[:120] + "..."
	}
	return collapsed
}

// deadlineFor prefers the caller's deadline over the session timeout, so a
// dispatch-level budget can shorten a conversation but never extend it.
func deadlineFor(ctx context.Context, session time.Duration) time.Time {
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(time.Now().Add(session)) {
		return deadline
	}
	return time.Now().Add(session)
}

func NewSender(cfg config.Config, logger *slog.Logger) Sender {
	if cfg.SMTPConfigured() {
		return NewSMTPSender(cfg, logger)
	}
	return NewLogSender(logger)
}

func DescribeTarget(message domain.MailMessage) string {
	if message.Email != "" {
		return message.Email
	}
	return message.UserID
}

func mimeEncoded(value string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	return "=?UTF-8?B?" + encoded + "?="
}
