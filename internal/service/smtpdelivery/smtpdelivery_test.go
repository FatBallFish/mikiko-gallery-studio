package smtpdelivery

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestSendVerificationCodeUsesImplicitTLS(t *testing.T) {
	addr, stop := startImplicitTLSSMTPServer(t)
	defer stop()
	host, port := splitHostPort(t, addr)
	markImplicitTLSPortForTest(t, port)

	cfg := config.SMTPConfig{
		Host:               host,
		Port:               port,
		Username:           "mailer@example.com",
		Password:           "smtp-password",
		From:               "Pic Gallery <noreply@example.com>",
		StartTLS:           true,
		InsecureSkipVerify: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := SendVerificationCode(ctx, cfg, "user@example.com", "smtp_test", "000000"); err != nil {
		t.Fatalf("SendVerificationCode: %v", err)
	}
}

func TestPort465DefaultsToImplicitTLS(t *testing.T) {
	if !usesImplicitTLS(config.SMTPConfig{Port: 465}) {
		t.Fatal("expected SMTP port 465 to use implicit TLS")
	}
}

func TestSafeFailureMessageClassifiesAuthenticationFailure(t *testing.T) {
	err := wrapFailure(StageAuth, fmt.Errorf("535 username@example.com rejected"))
	message := SafeFailureMessage(err)
	if !strings.Contains(message, "authentication failed") {
		t.Fatalf("expected actionable authentication message, got %q", message)
	}
	if strings.Contains(message, "username@example.com") {
		t.Fatalf("safe message exposed provider detail: %q", message)
	}
}

func markImplicitTLSPortForTest(t *testing.T, port int) {
	t.Helper()
	implicitTLSPorts[port] = struct{}{}
	t.Cleanup(func() { delete(implicitTLSPorts, port) })
}

func startImplicitTLSSMTPServer(t *testing.T) (string, func()) {
	t.Helper()
	cert, err := selfSignedCert()
	if err != nil {
		t.Fatalf("selfSignedCert: %v", err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go serveSMTPConn(conn)
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func serveSMTPConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeSMTPLine(writer, "220 localhost ESMTP")
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		if inData {
			if line == "." {
				inData = false
				writeSMTPLine(writer, "250 queued")
			}
			continue
		}
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			writeSMTPLine(writer, "250-localhost")
			writeSMTPLine(writer, "250 AUTH PLAIN")
		case strings.HasPrefix(upper, "HELO"):
			writeSMTPLine(writer, "250 localhost")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			if !validPlainAuth(line, "mailer@example.com", "smtp-password") {
				writeSMTPLine(writer, "535 invalid auth")
				continue
			}
			writeSMTPLine(writer, "235 authenticated")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			writeSMTPLine(writer, "250 sender ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			writeSMTPLine(writer, "250 recipient ok")
		case upper == "DATA":
			inData = true
			writeSMTPLine(writer, "354 end with dot")
		case upper == "QUIT":
			writeSMTPLine(writer, "221 bye")
			return
		default:
			writeSMTPLine(writer, "250 ok")
		}
	}
}

func writeSMTPLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}

func validPlainAuth(line, username, password string) bool {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return false
	}
	payload, err := base64.StdEncoding.DecodeString(parts[2])
	return err == nil && string(payload) == "\x00"+username+"\x00"+password
}

func selfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	return tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}),
	)
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}
