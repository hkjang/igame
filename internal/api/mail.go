package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/mail"
	"github.com/jackc/pgx/v5"
)

// mailConfig reads the `mail` setting with the password opened, which is the
// shape the transport needs. A deployment that never saved the setting has
// no row, and that is the default: off.
func (s *Server) mailConfig(ctx context.Context) (mail.Config, error) {
	var config mail.Config
	if err := s.setting(ctx, "mail", &config); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return mail.Config{}.Normalized(), nil
		}
		return mail.Config{}, err
	}
	config = config.Normalized()
	if config.Password != "" {
		plain, err := s.Secrets.Open(config.Password)
		if err != nil {
			return mail.Config{}, err
		}
		config.Password = plain
	}
	return config, nil
}

// lookupEmails is the one directory query mail borrows from the users table.
// Disabled accounts, accounts without an address and accounts whose
// preferences say mail_opt_out are left out, so the answer is exactly who
// may be written to.
func (s *Server) lookupEmails(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT u.id,u.email FROM users u
		LEFT JOIN user_preferences p ON p.user_id=u.id
		WHERE u.id=ANY($1) AND u.status='active' AND u.email<>''
		AND NOT COALESCE((p.value->>'mail_opt_out')::boolean,false)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	addresses := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		addresses[id] = email
	}
	return addresses, rows.Err()
}

// notifyMail sends one event mail in the background. Every caller is on a
// request path that has already succeeded, so nothing here reaches the user.
func (s *Server) notifyMail(r *http.Request, notification mail.Notification, recipients []uuid.UUID) {
	if s.mailer == nil || len(recipients) == 0 {
		return
	}
	p, _ := principalFrom(r)
	s.mailer.Notify(r.Context(), notification, p.UserID, recipients)
}

// reviewersFor is everyone who could act on a request from this requester:
// administrators, managers of the requester's team, and operators when the
// approval policy lets them review. The requester is dropped later, so a
// manager filing their own request is not told about it.
func (s *Server) reviewersFor(ctx context.Context, requesterTeam string, operatorsMayReview bool) ([]uuid.UUID, error) {
	rows, err := s.DB.Query(ctx, `SELECT id FROM users WHERE status='active' AND (role='admin' OR (role='manager' AND team<>'' AND team=$1) OR ($2 AND role='operator'))`,
		strings.TrimSpace(requesterTeam), operatorsMayReview)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// notifyReviewers mails the people who can review something the actor just
// submitted. A lookup failure is logged and the request goes on: the
// submission is already saved, and the review queue still shows it.
func (s *Server) notifyReviewers(r *http.Request, requesterTeam string, operatorsMayReview bool, notification mail.Notification) {
	if s.mailer == nil {
		return
	}
	reviewers, err := s.reviewersFor(r.Context(), requesterTeam, operatorsMayReview)
	if err != nil {
		s.logRequestError(r, err)
		return
	}
	s.notifyMail(r, notification, reviewers)
}

// creatorTeam is the team a content version is reviewed under: the one its
// creator belongs to, which is how the pending lists are filtered too.
func (s *Server) creatorTeam(ctx context.Context, creator *uuid.UUID) string {
	if creator == nil {
		return ""
	}
	var team string
	_ = s.DB.QueryRow(ctx, `SELECT team FROM users WHERE id=$1`, *creator).Scan(&team)
	return team
}

// versionTitle names a content version the way its screen does.
func versionTitle(number int, label string) string {
	if strings.TrimSpace(label) != "" {
		return fmt.Sprintf("v%d %s", number, strings.TrimSpace(label))
	}
	return fmt.Sprintf("v%d", number)
}

// principalLabel is the name people recognise for the person making the
// request, falling back to the username so a mail never shows a bare uuid.
func principalLabel(p Principal) string {
	if strings.TrimSpace(p.DisplayName) != "" {
		return strings.TrimSpace(p.DisplayName)
	}
	return p.Username
}

func (s *Server) getMailSetting(w http.ResponseWriter, r *http.Request) {
	var config mail.Config
	if err := s.setting(r.Context(), "mail", &config); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.dbError(w, r, err)
		return
	}
	redacted, configured := config.Normalized().Redacted()
	writeJSON(w, 200, map[string]any{"setting": redacted, "password_configured": configured})
}

// putMailSetting stores the relay setting. Like the OIDC client secret, the
// password is carried over when the screen sends none — the screen never
// receives it — and an unreadable current value stops the write rather than
// erasing a working password.
func (s *Server) putMailSetting(w http.ResponseWriter, r *http.Request) {
	var in mail.Config
	if !decodeJSON(w, r, &in) {
		return
	}
	in = in.Normalized()
	if err := in.Validate(); err != nil {
		writeError(w, 400, "invalid_mail", strings.TrimPrefix(err.Error(), mail.ErrInvalid.Error()+": "))
		return
	}
	var old mail.Config
	if err := s.setting(r.Context(), "mail", &old); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.serverError(w, r, 503, "mail_setting_unavailable", "current mail setting is unavailable", err)
		return
	}
	switch {
	case in.Username == "":
		// No username means no authentication; a password kept beside it
		// would only be a secret nobody uses.
		in.Password = ""
	case in.Password == "" || in.Password == "********":
		in.Password = old.Password
	default:
		sealed, err := s.Secrets.Seal(in.Password)
		if err != nil {
			s.dbError(w, r, err)
			return
		}
		in.Password = sealed
	}
	raw, _ := encodeSetting(in)
	p, _ := principalFrom(r)
	_, err := s.DB.Exec(r.Context(), `INSERT INTO system_settings(key,value,secret,updated_by) VALUES('mail',$1,true,$2)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value,secret=true,updated_by=excluded.updated_by,updated_at=now()`, raw, p.UserID)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.invalidateSetting("mail")
	previous, _ := json.Marshal(old)
	s.audit(r, "setting.update", "setting", "mail", auditSettingChange(previous, raw))
	redacted, configured := in.Redacted()
	writeJSON(w, 200, map[string]any{"setting": redacted, "password_configured": configured})
}

// listMailDeliveries shows what left the building, newest first.
func (s *Server) listMailDeliveries(w http.ResponseWriter, r *http.Request) {
	if s.mailer == nil {
		writeJSON(w, 200, mail.Page{Items: []mail.Delivery{}, Summary: mail.Summary{Status: map[string]int{}}})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := s.mailer.Deliveries(r.Context(), r.URL.Query().Get("status"), limit)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	writeJSON(w, 200, page)
}

// sendTestMail proves the saved relay setting works before anything depends
// on it. The recipient defaults to the administrator pressing the button.
func (s *Server) sendTestMail(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeOptionalJSON(w, r, &input) {
		return
	}
	p, _ := principalFrom(r)
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(p.Email)
	}
	if _, err := netmail.ParseAddress(recipient); err != nil || recipient == "" {
		writeError(w, 400, "invalid_recipient", "recipient must be an email address")
		return
	}
	if s.mailer == nil {
		writeError(w, 503, "mail_unavailable", "mail is not configured on this server")
		return
	}
	err := s.mailer.SendNow(r.Context(), mail.TestMessage(), p.UserID, recipient)
	switch {
	case errors.Is(err, mail.ErrDisabled):
		writeError(w, 409, "mail_disabled", "enable mail and save the setting before sending a test")
		return
	case err != nil:
		// The relay's own words go back to the screen: "connection refused"
		// and "535 authentication failed" call for different fixes.
		s.audit(r, "mail.test", "mail", recipient, map[string]any{"sent": false, "error": err.Error()})
		writeError(w, 502, "mail_send_failed", err.Error())
		return
	}
	s.audit(r, "mail.test", "mail", recipient, map[string]any{"sent": true})
	writeJSON(w, 200, map[string]any{"sent": true, "recipient": recipient})
}
