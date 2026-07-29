package smtpdelivery

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type Stage string

const (
	StageConnect   Stage = "connect"
	StageTLS       Stage = "tls"
	StageProtocol  Stage = "protocol"
	StageHello     Stage = "hello"
	StageAuth      Stage = "auth"
	StageMailFrom  Stage = "mail_from"
	StageRecipient Stage = "recipient"
	StageData      Stage = "data"
)

const defaultTimeout = 10 * time.Second

var implicitTLSPorts = map[int]struct{}{465: {}}

type Failure struct {
	Stage Stage
	Err   error
}

func (e *Failure) Error() string {
	return fmt.Sprintf("smtp %s: %v", e.Stage, e.Err)
}

func (e *Failure) Unwrap() error {
	return e.Err
}

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
		return wrapFailure(StageMailFrom, err)
	}
	if err := client.Rcpt(email); err != nil {
		return wrapFailure(StageRecipient, err)
	}
	writer, err := client.Data()
	if err != nil {
		return wrapFailure(StageData, err)
	}
	message := VerificationEmailMessage(cfg.From, email, scene, code)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return wrapFailure(StageData, err)
	}
	if err := writer.Close(); err != nil {
		return wrapFailure(StageData, err)
	}
	if err := client.Quit(); err != nil {
		return wrapFailure(StageProtocol, err)
	}
	return nil
}

func Configured(cfg config.SMTPConfig) bool {
	return strings.TrimSpace(cfg.Host) != "" && cfg.Port > 0 && strings.TrimSpace(cfg.From) != ""
}

func ConfigError() error {
	return fmt.Errorf("email verification SMTP delivery is not configured: set auth.smtp.host, auth.smtp.port, and auth.smtp.from")
}

func FailureStage(err error) Stage {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Stage
	}
	return StageProtocol
}

func SafeFailureMessage(err error) string {
	switch FailureStage(err) {
	case StageConnect:
		return "SMTP server connection failed; check the host, port, DNS, firewall, and outbound network policy"
	case StageTLS:
		return "SMTP TLS negotiation failed; port 465 uses implicit TLS, while port 587 normally uses STARTTLS"
	case StageAuth:
		return "SMTP authentication failed; check the username and provider authorization code or password"
	case StageMailFrom:
		return "SMTP sender was rejected; check that From is an address authorized by the provider"
	case StageRecipient:
		return "SMTP recipient was rejected; check the test email address and provider delivery policy"
	case StageData:
		return "SMTP server rejected the test message data"
	default:
		return "SMTP protocol negotiation failed; check the server port and encryption settings"
	}
}

func VerificationEmailMessage(from, to, scene, code string) string {
	subject := "Pic Gallery verification code"
	body := fmt.Sprintf("Your Pic Gallery verification code is %s. It expires in 10 minutes.", code)
	if scene = strings.TrimSpace(scene); scene != "" {
		body = fmt.Sprintf("Your Pic Gallery verification code for %s is %s. It expires in 10 minutes.", scene, code)
	}
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(to),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n"
}

func newReadyClient(ctx context.Context, cfg config.SMTPConfig) (*smtp.Client, error) {
	client, err := newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := client.Hello("localhost"); err != nil {
		_ = client.Close()
		return nil, wrapFailure(StageHello, err)
	}
	if cfg.StartTLS && !usesImplicitTLS(cfg) {
		if err := client.StartTLS(tlsConfig(cfg)); err != nil {
			_ = client.Close()
			return nil, wrapFailure(StageTLS, err)
		}
	}
	if strings.TrimSpace(cfg.Username) != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, wrapFailure(StageAuth, err)
		}
	}
	return client, nil
}

func newClient(ctx context.Context, cfg config.SMTPConfig) (*smtp.Client, error) {
	ctx, cancel := withDefaultTimeout(ctx)
	defer cancel()
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, wrapFailure(StageConnect, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close()
			return nil, wrapFailure(StageConnect, err)
		}
	}
	if usesImplicitTLS(cfg) {
		tlsConn := tls.Client(conn, tlsConfig(cfg))
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, wrapFailure(StageTLS, err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, wrapFailure(StageProtocol, err)
	}
	return client, nil
}

func withDefaultTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

func tlsConfig(cfg config.SMTPConfig) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         cfg.Host,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
}

func usesImplicitTLS(cfg config.SMTPConfig) bool {
	_, ok := implicitTLSPorts[cfg.Port]
	return ok
}

func wrapFailure(stage Stage, err error) error {
	return &Failure{Stage: stage, Err: err}
}

func envelopeAddress(value string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return strings.TrimSpace(value)
	}
	return parsed.Address
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	return strings.ReplaceAll(value, "\n", "")
}
