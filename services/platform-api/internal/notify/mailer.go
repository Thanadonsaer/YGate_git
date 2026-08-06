// Package notify sends operator-facing email notifications (alarm breaches)
// over SMTP. It mirrors the STARTTLS transport auth-service's password-reset
// notifier uses, generalized for arbitrary subject/body and multiple
// recipients since the two services are separate Go modules and can't share
// an internal package.
package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Mailer struct {
	addr, from, username, password string
}

// NewMailer returns nil (a valid, always-disabled *Mailer) when addr is
// blank, so callers can construct unconditionally from config and just check
// Enabled() rather than threading an extra bool through the call chain.
func NewMailer(addr, from, username, password string) *Mailer {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	return &Mailer{addr: addr, from: from, username: username, password: password}
}

func (m *Mailer) Enabled() bool { return m != nil }

// Send delivers a themed HTML email (see RenderEmail) to every recipient in
// one SMTP session. Recipient and subject values are checked for
// header-injection characters since they can originate from user-entered
// alarm rule labels and account emails.
func (m *Mailer) Send(ctx context.Context, recipients []string, subject, html string) error {
	if !m.Enabled() || len(recipients) == 0 {
		return nil
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("invalid email subject")
	}
	for _, recipient := range recipients {
		if strings.ContainsAny(recipient, "\r\n") {
			return fmt.Errorf("invalid email recipient")
		}
	}
	message := []byte("From: " + m.from + "\r\n" +
		"To: " + strings.Join(recipients, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + html + "\r\n")
	return m.sendSTARTTLS(ctx, recipients, message)
}

func (m *Mailer) sendSTARTTLS(ctx context.Context, recipients []string, message []byte) error {
	host, _, _ := net.SplitHostPort(m.addr)
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", m.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(10 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	// ponytail: STARTTLS only (matches auth-service's notifier); add implicit-TLS
	// (port 465) support if a deployment's SMTP relay needs it.
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("SMTP server does not support STARTTLS")
	}
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if m.username != "" {
		if err = client.Auth(smtp.PlainAuth("", m.username, m.password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(m.from); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
