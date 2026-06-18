package smtpdelivery

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

var implicitTLSPorts = map[int]struct{}{465: {}}

func SendVerificationCode(ctx context.Context, cfg config.SMTPConfig, email, scene, code string) error {
	if !Configured(cfg) {
		return ConfigError()
	}
	client, err := newReadyClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Mail(envelopeAddress(cfg.From)); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(email); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	message := VerificationEmailMessage(cfg.From, email, scene, code)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write smtp data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close smtp data: %w", err)
	}
	return client.Quit()
}

func ValidateConnectivity(ctx context.Context, cfg config.SMTPConfig) error {
	if !Configured(cfg) {
		return ConfigError()
	}
	client, err := newReadyClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Quit()
}

func Configured(cfg config.SMTPConfig) bool {
	return strings.TrimSpace(cfg.Host) != "" && cfg.Port > 0 && strings.TrimSpace(cfg.From) != ""
}

func ConfigError() error {
	return fmt.Errorf("email verification SMTP delivery is not configured: set auth.smtp.host, auth.smtp.port, and auth.smtp.from")
}

func VerificationEmailMessage(from, to, scene, code string) string {
	subject := "Pic Gallery Verification Code"
	if scene = strings.TrimSpace(scene); scene != "" {
		subject = "Pic Gallery " + scene + " Code"
	}
	fromAddress := mail.Address{Name: "Pic Gallery", Address: envelopeAddress(from)}
	fromHeader := fromAddress.String()
	if parsed, err := mail.ParseAddress(from); err == nil {
		fromHeader = parsed.String()
	}
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour verification code is %s. It expires in 10 minutes.\r\n", fromHeader, to, subject, code)
}

func newReadyClient(ctx context.Context, cfg config.SMTPConfig) (*smtp.Client, error) {
	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := client.Hello("localhost"); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("smtp hello: %w", err)
	}
	if cfg.StartTLS && !usesImplicitTLS(cfg) {
		if err := client.StartTLS(tlsConfig(cfg)); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Username) != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("smtp auth: %w", err)
		}
	}
	return client, nil
}

func newClient(ctx context.Context, cfg config.SMTPConfig) (*smtp.Client, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}
	var (
		conn net.Conn
		err  error
	)
	if usesImplicitTLS(cfg) {
		dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: tlsConfig(cfg)}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	} else {
		dialer := net.Dialer{Timeout: 10 * time.Second}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("connect smtp server: %w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create smtp client: %w", err)
	}
	return client, nil
}

func tlsConfig(cfg config.SMTPConfig) *tls.Config {
	return &tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.InsecureSkipVerify}
}

func usesImplicitTLS(cfg config.SMTPConfig) bool {
	_, ok := implicitTLSPorts[cfg.Port]
	return ok
}

func envelopeAddress(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return strings.TrimSpace(value)
}
