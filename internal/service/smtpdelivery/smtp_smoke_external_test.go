package smtpdelivery

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestExternalSMTPFromLocalSecret(t *testing.T) {
	if os.Getenv("PIC_GALLERY_SMTP_SMOKE") != "1" {
		t.Skip("set PIC_GALLERY_SMTP_SMOKE=1 to run external SMTP smoke")
	}
	path := strings.TrimSpace(os.Getenv("PIC_GALLERY_SMTP_SMOKE_FILE"))
	if path == "" {
		t.Skip("set PIC_GALLERY_SMTP_SMOKE_FILE to a local SMTP config file")
	}
	cfg, recipient := loadSMTPConfigForSmoke(t, path)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := SendVerificationCode(ctx, cfg, recipient, "smtp_test", "000000"); err != nil {
		t.Fatalf("SendVerificationCode external SMTP smoke: %v", err)
	}
}

func loadSMTPConfigForSmoke(t *testing.T, path string) (config.SMTPConfig, string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open smtp config: %v", err)
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, ok := strings.Cut(line, "：")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
		}
		if !ok {
			key, value, ok = strings.Cut(line, "=")
		}
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan smtp config: %v", err)
	}
	port, _ := strconv.Atoi(firstSmokeValue(values, "端口", "port", "smtp_port"))
	return config.SMTPConfig{
		Host:     firstSmokeValue(values, "主机", "host", "smtp_host"),
		Port:     port,
		Username: firstSmokeValue(values, "smtp用户名", "username", "user", "smtp_username"),
		Password: firstSmokeValue(values, "smtp密码/授权码", "password", "pass", "smtp_password"),
		From:     firstSmokeValue(values, "发件人邮箱", "from", "smtp_from", "smtp用户名", "username"),
		StartTLS: strings.EqualFold(firstSmokeValue(values, "使用tls", "tls", "starttls", "smtp_starttls"), "true"),
	}, firstSmokeValue(values, "测试收件邮箱", "test_email")
}

func firstSmokeValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[strings.ToLower(key)]); value != "" {
			return value
		}
	}
	return ""
}
