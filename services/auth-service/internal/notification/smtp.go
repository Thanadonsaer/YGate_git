package notification

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type SMTPResetNotifier struct {
	addr, from, username, password, resetURL string
}

func NewSMTPResetNotifier(addr, from, username, password, resetURL string) (*SMTPResetNotifier, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("invalid SMTP address: %w", err)
	}
	if strings.ContainsAny(from, "\r\n") {
		return nil, fmt.Errorf("invalid SMTP sender")
	}
	parsed, err := url.Parse(resetURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("password reset URL must use HTTPS")
	}
	return &SMTPResetNotifier{addr: addr, from: from, username: username, password: password, resetURL: resetURL}, nil
}

func (n *SMTPResetNotifier) Notify(ctx context.Context, recipient, token string) error {
	if strings.ContainsAny(recipient, "\r\n") {
		return fmt.Errorf("invalid SMTP recipient")
	}
	resetURL, err := url.Parse(n.resetURL)
	if err != nil {
		return err
	}
	query := resetURL.Query()
	query.Set("token", token)
	resetURL.RawQuery = query.Encode()
	html, err := renderEmail(emailContent{
		Title:       "Reset your password",
		Body:        "We received a request to reset your Solar SCADA password. This link expires soon and can only be used once.",
		ButtonURL:   resetURL.String(),
		ButtonLabel: "Reset password",
	})
	if err != nil {
		return fmt.Errorf("render password reset email: %w", err)
	}
	message := []byte("To: " + recipient + "\r\n" +
		"From: " + n.from + "\r\n" +
		"Subject: Reset your Solar SCADA password\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + html)
	if err = n.sendSTARTTLS(ctx, recipient, message); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	return nil
}

func (n *SMTPResetNotifier) sendSTARTTLS(ctx context.Context, recipient string, message []byte) error {
	host, _, _ := net.SplitHostPort(n.addr)
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", n.addr)
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
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("SMTP server does not support STARTTLS")
	}
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if n.username != "" {
		if err = client.Auth(smtp.PlainAuth("", n.username, n.password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(n.from); err != nil {
		return err
	}
	if err = client.Rcpt(recipient); err != nil {
		return err
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

func (n *SMTPResetNotifier) NotifyEmailVerification(ctx context.Context, recipient, token string) error {
	if strings.ContainsAny(recipient, "\r\n") {
		return fmt.Errorf("invalid SMTP recipient")
	}
	resetURL, err := url.Parse(n.resetURL)
	if err != nil {
		return err
	}
	query := resetURL.Query()
	query.Set("verify", token)
	resetURL.RawQuery = query.Encode()
	html, err := renderEmail(emailContent{
		Title:       "Verify your account email",
		Body:        "Confirm this email address to finish setting up your Solar SCADA account.",
		ButtonURL:   resetURL.String(),
		ButtonLabel: "Verify email",
	})
	if err != nil {
		return fmt.Errorf("render email verification: %w", err)
	}
	message := []byte("To: " + recipient + "\r\n" +
		"From: " + n.from + "\r\n" +
		"Subject: Verify your Solar SCADA account email\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + html)
	if err = n.sendSTARTTLS(ctx, recipient, message); err != nil {
		return fmt.Errorf("send email verification: %w", err)
	}
	return nil
}
