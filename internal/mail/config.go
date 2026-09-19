// Package mail delivers event notifications through a company SMTP relay.
//
// Internal relays commonly accept mail on port 25 with no credentials and no
// TLS, so authentication and encryption are optional and the transport adapts
// to whatever the server advertises. Nothing here blocks a request: sending
// happens in the background and every attempt is recorded so an administrator
// can see what left the building.
package mail

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrDisabled = errors.New("mail is disabled")
	ErrInvalid  = errors.New("invalid mail configuration")
)

// Defaults aim at the common case: an internal relay on port 25 that accepts
// mail from the network without credentials.
const (
	DefaultPort     = 25
	DefaultSecurity = "auto"
	DefaultTimeout  = 10
)

// Event names. Each one has a switch in the setting, notify_<event>, so an
// administrator can silence one kind without turning the whole thing off.
const (
	EventApprovalRequested = "approval_requested"
	EventApprovalDecided   = "approval_decided"
	EventRankingModerated  = "ranking_moderated"
	EventTest              = "test"
)

// Events lists the switchable kinds in the order the settings screen shows
// them. The test mail has no switch: it is sent on purpose, by hand.
var Events = []string{EventApprovalRequested, EventApprovalDecided, EventRankingModerated}

// Config is the stored `mail` setting. The field names are the standard's
// keys with the `mail.` prefix dropped, so the same setting reads as
// mail.smtp_host here and in every other service that follows it.
type Config struct {
	Enabled       bool   `json:"enabled"`
	SMTPHost      string `json:"smtp_host"`
	SMTPPort      int    `json:"smtp_port"`
	Security      string `json:"security"`
	SkipTLSVerify bool   `json:"skip_tls_verify"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	FromAddress   string `json:"from_address"`
	FromName      string `json:"from_name"`
	BaseURL       string `json:"base_url"`
	Timeout       int    `json:"timeout_seconds"`
	// Per-event switches. A pointer so an absent key means "on": adding a
	// notification never requires a settings change first.
	NotifyApprovalRequested *bool `json:"notify_approval_requested,omitempty"`
	NotifyApprovalDecided   *bool `json:"notify_approval_decided,omitempty"`
	NotifyRankingModerated  *bool `json:"notify_ranking_moderated,omitempty"`
}

// Normalized trims what an administrator typed and fills the defaults the
// relay case needs, without touching the password.
func (c Config) Normalized() Config {
	c.SMTPHost = strings.TrimSpace(c.SMTPHost)
	c.Security = strings.ToLower(strings.TrimSpace(c.Security))
	c.Username = strings.TrimSpace(c.Username)
	c.FromAddress = strings.TrimSpace(c.FromAddress)
	c.FromName = strings.TrimSpace(c.FromName)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.SMTPPort == 0 {
		c.SMTPPort = DefaultPort
	}
	if c.Security == "" {
		c.Security = DefaultSecurity
	}
	// A relay on the implicit TLS port needs no extra configuration.
	if c.Security == DefaultSecurity && c.SMTPPort == 465 {
		c.Security = "tls"
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

// Validate reports what stops the setting from being saved. The address
// fields are checked whether or not mail is on, so a typo is caught while the
// administrator is looking at the screen rather than at the first send.
func (c Config) Validate() error {
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return fmt.Errorf("%w: smtp_port must be between 1 and 65535", ErrInvalid)
	}
	switch c.Security {
	case "auto", "none", "starttls", "tls":
	default:
		return fmt.Errorf("%w: security must be auto, none, starttls, or tls", ErrInvalid)
	}
	if c.Timeout < 1 || c.Timeout > 300 {
		return fmt.Errorf("%w: timeout_seconds must be between 1 and 300", ErrInvalid)
	}
	if c.FromAddress != "" {
		if _, err := mail.ParseAddress(c.FromAddress); err != nil {
			return fmt.Errorf("%w: from_address must be an email address", ErrInvalid)
		}
	}
	if c.BaseURL != "" && !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("%w: base_url must be an absolute HTTP(S) URL", ErrInvalid)
	}
	if !c.Enabled {
		return nil
	}
	if c.SMTPHost == "" {
		return fmt.Errorf("%w: smtp_host is required when mail is enabled", ErrInvalid)
	}
	if c.FromAddress == "" {
		return fmt.Errorf("%w: from_address is required when mail is enabled", ErrInvalid)
	}
	return nil
}

// Allows reports whether an event should be delivered. Unknown events and the
// test mail are always allowed.
func (c Config) Allows(event string) bool {
	var flag *bool
	switch event {
	case EventApprovalRequested:
		flag = c.NotifyApprovalRequested
	case EventApprovalDecided:
		flag = c.NotifyApprovalDecided
	case EventRankingModerated:
		flag = c.NotifyRankingModerated
	}
	return flag == nil || *flag
}

// Address is the RFC 5322 From header value.
func (c Config) Address() string {
	if c.FromName != "" {
		return fmt.Sprintf("%s <%s>", c.FromName, c.FromAddress)
	}
	return c.FromAddress
}

func (c Config) endpoint() string { return net.JoinHostPort(c.SMTPHost, fmt.Sprint(c.SMTPPort)) }

func (c Config) timeout() time.Duration {
	if c.Timeout <= 0 {
		return DefaultTimeout * time.Second
	}
	return time.Duration(c.Timeout) * time.Second
}

// Redacted is the config as the settings API returns it: the password is
// replaced by whether one is stored, and never the value.
func (c Config) Redacted() (Config, bool) {
	configured := c.Password != ""
	c.Password = ""
	return c, configured
}
