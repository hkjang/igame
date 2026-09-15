package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Message is one mail as it goes onto the wire.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Deliver opens a connection and sends one message. It is exported so the
// settings screen can prove the relay works before anything depends on it.
func Deliver(ctx context.Context, config Config, message Message) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if config.SMTPHost == "" || config.FromAddress == "" {
		return fmt.Errorf("%w: smtp_host and from_address are required", ErrInvalid)
	}
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("%w: recipient is required", ErrInvalid)
	}
	client, err := dial(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err := startSession(client, config); err != nil {
		return err
	}
	if err := client.Mail(config.FromAddress); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(strings.TrimSpace(message.To)); err != nil {
		return fmt.Errorf("RCPT TO failed: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}
	if _, err := writer.Write([]byte(compose(config, message, time.Now()))); err != nil {
		return fmt.Errorf("body failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("body was not accepted: %w", err)
	}
	return client.Quit()
}

func dial(ctx context.Context, config Config) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: config.timeout()}
	var connection net.Conn
	var err error
	if config.Security == "tls" {
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: config.tlsConfig()}).DialContext(ctx, "tcp", config.endpoint())
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", config.endpoint())
	}
	if err != nil {
		return nil, fmt.Errorf("SMTP connect failed: %w", err)
	}
	// The deadline covers the whole dialogue, so a relay that accepts the
	// connection and then goes quiet cannot hold a goroutine forever.
	_ = connection.SetDeadline(time.Now().Add(config.timeout()))
	client, err := smtp.NewClient(connection, config.SMTPHost)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("SMTP greeting failed: %w", err)
	}
	return client, nil
}

// startSession upgrades and authenticates only as far as the relay allows, so
// an unauthenticated internal relay works with the same settings as a hosted
// provider that demands both.
func startSession(client *smtp.Client, config Config) error {
	if err := client.Hello(helloName(config)); err != nil {
		return fmt.Errorf("EHLO failed: %w", err)
	}
	if config.Security == "starttls" || config.Security == "auto" {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(config.tlsConfig()); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		} else if config.Security == "starttls" {
			return fmt.Errorf("%w: the server does not offer STARTTLS", ErrInvalid)
		}
	}
	if config.Username == "" {
		return nil
	}
	supported, mechanisms := client.Extension("AUTH")
	if !supported {
		return fmt.Errorf("%w: the server does not offer authentication; leave username empty", ErrInvalid)
	}
	upper := strings.ToUpper(mechanisms)
	switch {
	case strings.Contains(upper, "PLAIN"):
		return client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.SMTPHost))
	case strings.Contains(upper, "LOGIN"):
		return client.Auth(loginAuth{username: config.Username, password: config.Password, host: config.SMTPHost})
	default:
		return client.Auth(smtp.CRAMMD5Auth(config.Username, config.Password))
	}
}

func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: c.SMTPHost, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.SkipTLSVerify} //nolint:gosec // opt-in for internal relays with private certificates
}

// helloName keeps the EHLO name to the sender domain, which relays that check
// the greeting are happier with than a container hostname.
func helloName(config Config) string {
	if _, domain, found := strings.Cut(config.FromAddress, "@"); found && domain != "" {
		return domain
	}
	return "localhost"
}

// loginAuth implements the LOGIN mechanism that several corporate relays use
// instead of PLAIN. The standard library only ships PLAIN and CRAM-MD5.
type loginAuth struct{ username, password, host string }

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && server.Name != a.host {
		return "", nil, errors.New("LOGIN authentication is only used with the configured server")
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimRight(string(fromServer), ": ")) {
	case "username":
		return []byte(a.username), nil
	case "password":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected LOGIN challenge: %s", fromServer)
}

// compose builds a MIME message. Korean subjects and names are encoded so
// relays and clients that predate UTF-8 headers still show them correctly.
func compose(config Config, message Message, now time.Time) string {
	var builder strings.Builder
	builder.WriteString("From: " + encodeAddress(config.Address()) + "\r\n")
	builder.WriteString("To: " + strings.TrimSpace(message.To) + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", message.Subject) + "\r\n")
	builder.WriteString("Date: " + now.Format(time.RFC1123Z) + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	builder.WriteString("Auto-Submitted: auto-generated\r\n")
	builder.WriteString("X-Igame-Notification: 1\r\n")
	builder.WriteString("\r\n")
	// The DATA writer net/smtp hands out already turns bare newlines into CRLF
	// and doubles a leading dot, so the body goes in as written; escaping it
	// here as well would put a second dot on the wire.
	builder.WriteString(message.Body)
	if !strings.HasSuffix(message.Body, "\n") {
		builder.WriteString("\n")
	}
	return builder.String()
}

func encodeAddress(address string) string {
	open := strings.LastIndex(address, "<")
	if open <= 0 {
		return address
	}
	return mime.QEncoding.Encode("utf-8", strings.TrimSpace(address[:open])) + " " + address[open:]
}
