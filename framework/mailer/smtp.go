package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Config is the outbound SMTP connection used for dashboard emails.
type Config struct {
	Enabled   bool
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
	UseTLS    bool
}

// Message is a simple text email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Send delivers a plaintext email via SMTP. Returns error if SMTP is disabled or misconfigured.
func Send(cfg Config, msg Message) error {
	if !cfg.Enabled {
		return fmt.Errorf("SMTP is not enabled")
	}
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return fmt.Errorf("SMTP host is required")
	}
	port := cfg.Port
	if port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(cfg.FromEmail)
	if from == "" {
		from = strings.TrimSpace(cfg.Username)
	}
	if from == "" {
		return fmt.Errorf("SMTP from email is required")
	}
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return fmt.Errorf("recipient email is required")
	}

	fromHeader := from
	if name := strings.TrimSpace(cfg.FromName); name != "" {
		fromHeader = fmt.Sprintf("%s <%s>", name, from)
	}

	payload := strings.Join([]string{
		"From: " + fromHeader,
		"To: " + to,
		"Subject: " + msg.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		msg.Body,
	}, "\r\n")

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, host)

	if cfg.UseTLS || port == 465 {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		var conn net.Conn
		var err error
		if port == 465 {
			conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsCfg)
		} else {
			conn, err = net.DialTimeout("tcp", addr, 15*time.Second)
		}
		if err != nil {
			return fmt.Errorf("smtp dial: %w", err)
		}
		defer conn.Close()

		var client *smtp.Client
		if port == 465 {
			client, err = smtp.NewClient(conn, host)
		} else {
			client, err = smtp.NewClient(conn, host)
			if err == nil {
				if ok, _ := client.Extension("STARTTLS"); ok || cfg.UseTLS {
					if err = client.StartTLS(tlsCfg); err != nil {
						_ = client.Close()
						return fmt.Errorf("smtp starttls: %w", err)
					}
				}
			}
		}
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

		if cfg.Username != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := client.Mail(from); err != nil {
			return fmt.Errorf("smtp mail: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data: %w", err)
		}
		if _, err := w.Write([]byte(payload)); err != nil {
			_ = w.Close()
			return fmt.Errorf("smtp write: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close: %w", err)
		}
		return client.Quit()
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(payload))
}
