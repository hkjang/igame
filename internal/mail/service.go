package mail

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConfigSource reads the stored setting with the password already opened.
type ConfigSource func(context.Context) (Config, error)

// Directory resolves account ids to email addresses. The users table already
// knows every address, so mail keeps no list of its own; an account without
// an address, or one that asked not to be mailed, is simply absent from the
// answer.
type Directory func(context.Context, []uuid.UUID) (map[uuid.UUID]string, error)

// Delivery is one attempt to send one mail. It carries the subject and the
// recipient but never the body: the record answers "did it go out", and a
// table that also held the text would be a copy of every notification.
type Delivery struct {
	ID           uuid.UUID  `json:"id"`
	Event        string     `json:"event"`
	Recipient    string     `json:"recipient"`
	Subject      string     `json:"subject"`
	ResourceType string     `json:"resource_type,omitempty"`
	ResourceID   string     `json:"resource_id,omitempty"`
	ActorID      *uuid.UUID `json:"actor_id,omitempty"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Summary struct {
	Total  int            `json:"total"`
	Status map[string]int `json:"status"`
}

type Page struct {
	Items   []Delivery `json:"items"`
	Summary Summary    `json:"summary"`
}

type Service struct {
	pool      *pgxpool.Pool
	config    ConfigSource
	directory Directory
	logger    *slog.Logger
	now       func() time.Time
	send      func(context.Context, Config, Message) error
	inflight  sync.WaitGroup
}

func NewService(pool *pgxpool.Pool, config ConfigSource, directory Directory, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{pool: pool, config: config, directory: directory, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, send: Deliver}
}

// SetSender replaces the transport, which lets tests drive the service
// without a relay.
func (s *Service) SetSender(sender func(context.Context, Config, Message) error) { s.send = sender }

// Wait blocks until every background send has finished. Shutdown calls it so
// a mail that was queued is not lost with the process; tests call it to
// observe the outcome.
func (s *Service) Wait() { s.inflight.Wait() }

// Notify sends one event mail to each recipient in the background. The
// request that caused the event has returned to its caller long before the
// relay answers, and a relay that is down is a failed delivery record, not a
// failed request. The actor never receives mail about their own action.
func (s *Service) Notify(ctx context.Context, notification Notification, actor uuid.UUID, recipients []uuid.UUID) {
	if s == nil || s.config == nil || len(recipients) == 0 {
		return
	}
	config, err := s.config(ctx)
	if err != nil {
		s.logger.Warn("mail setting could not be read; notification skipped", "event", notification.Event, "error", err)
		return
	}
	if !config.Enabled || !config.Allows(notification.Event) {
		return
	}
	wanted := make([]uuid.UUID, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient == uuid.Nil || recipient == actor {
			continue
		}
		wanted = append(wanted, recipient)
	}
	if len(wanted) == 0 {
		return
	}
	// Everything after the switch check leaves the request path, including the
	// directory lookup: a slow database would otherwise make the request wait
	// for something it does not need.
	background := context.WithoutCancel(ctx)
	s.inflight.Add(1)
	go func() {
		defer s.inflight.Done()
		s.dispatch(background, config, notification, actor, wanted)
	}()
}

func (s *Service) dispatch(ctx context.Context, config Config, notification Notification, actor uuid.UUID, recipients []uuid.UUID) {
	addresses := s.resolve(ctx, recipients)
	if len(addresses) == 0 {
		return
	}
	body := notification.Render(config)
	var actorID *uuid.UUID
	if actor != uuid.Nil {
		actorID = &actor
	}
	for _, address := range addresses {
		delivery := Delivery{ID: uuid.New(), Event: notification.Event, Recipient: address, Subject: notification.Subject,
			ResourceType: notification.ResourceType, ResourceID: notification.ResourceID, ActorID: actorID, Status: "queued", CreatedAt: s.now()}
		s.record(ctx, delivery)
		s.deliver(ctx, delivery, config, Message{To: address, Subject: notification.Subject, Body: body})
	}
}

// SendNow delivers immediately and reports the outcome, which is what the
// administrator's test button needs.
func (s *Service) SendNow(ctx context.Context, notification Notification, actor uuid.UUID, recipient string) error {
	config, err := s.config(ctx)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return ErrDisabled
	}
	var actorID *uuid.UUID
	if actor != uuid.Nil {
		actorID = &actor
	}
	delivery := Delivery{ID: uuid.New(), Event: notification.Event, Recipient: recipient, Subject: notification.Subject,
		ActorID: actorID, Status: "queued", CreatedAt: s.now()}
	s.record(ctx, delivery)
	sendContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.timeout()+5*time.Second)
	defer cancel()
	delivery.Attempts = 1
	err = s.send(sendContext, config, Message{To: recipient, Subject: notification.Subject, Body: notification.Render(config)})
	s.complete(sendContext, delivery, err)
	return err
}

// deliver retries once, because a relay that briefly refuses a connection is
// common and losing the notification is worse than a short wait.
func (s *Service) deliver(ctx context.Context, delivery Delivery, config Config, message Message) {
	ctx, cancel := context.WithTimeout(ctx, 2*config.timeout()+15*time.Second)
	defer cancel()
	var err error
	for attempt := 1; attempt <= 2; attempt++ {
		delivery.Attempts = attempt
		if err = s.send(ctx, config, message); err == nil {
			break
		}
		if attempt == 1 {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	s.complete(ctx, delivery, err)
}

func (s *Service) complete(ctx context.Context, delivery Delivery, cause error) {
	status, message := "sent", ""
	if cause != nil {
		status, message = "failed", cause.Error()
		s.logger.Warn("notification mail failed", "event", delivery.Event, "recipient", delivery.Recipient, "attempts", delivery.Attempts, "error", cause)
	}
	if s.pool == nil {
		return
	}
	if _, err := s.pool.Exec(ctx, `UPDATE mail_deliveries SET status=$2,attempts=GREATEST(attempts,$3),error_message=$4,updated_at=$5 WHERE id=$1`,
		delivery.ID, status, max(delivery.Attempts, 1), truncate(message, 1000), s.now()); err != nil {
		s.logger.Warn("mail delivery status was not recorded", "error", err)
	}
}

func (s *Service) record(ctx context.Context, delivery Delivery) {
	if s.pool == nil {
		return
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO mail_deliveries(id,event,recipient,subject,resource_type,resource_id,actor_id,status,attempts,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,'queued',0,$8,$8)`,
		delivery.ID, delivery.Event, delivery.Recipient, truncate(delivery.Subject, 300), delivery.ResourceType, delivery.ResourceID, delivery.ActorID, delivery.CreatedAt); err != nil {
		s.logger.Warn("mail delivery was not recorded", "error", err)
	}
}

// resolve turns account ids into unique addresses. One person who is both an
// administrator and the requester's manager gets one mail, not two.
func (s *Service) resolve(ctx context.Context, recipients []uuid.UUID) []string {
	if s.directory == nil {
		return nil
	}
	emails, err := s.directory(ctx, recipients)
	if err != nil {
		s.logger.Warn("mail recipients were not resolved", "error", err)
		return nil
	}
	seen, addresses := map[string]struct{}{}, make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		address := strings.TrimSpace(emails[recipient])
		if address == "" {
			continue
		}
		key := strings.ToLower(address)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}

// Deliveries lists what was sent, newest first, with a status breakdown.
func (s *Service) Deliveries(ctx context.Context, status string, limit int) (Page, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	page := Page{Items: []Delivery{}, Summary: Summary{Status: map[string]int{}}}
	rows, err := s.pool.Query(ctx, `SELECT id,event,recipient,subject,resource_type,resource_id,actor_id,status,attempts,error_message,created_at,updated_at
		FROM mail_deliveries WHERE ($1='' OR status=$1) ORDER BY created_at DESC, id LIMIT $2`, strings.TrimSpace(status), limit)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Delivery
		if err := rows.Scan(&item.ID, &item.Event, &item.Recipient, &item.Subject, &item.ResourceType, &item.ResourceID, &item.ActorID,
			&item.Status, &item.Attempts, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	counts, err := s.pool.Query(ctx, `SELECT status, count(*) FROM mail_deliveries GROUP BY 1`)
	if err != nil {
		return Page{}, err
	}
	defer counts.Close()
	for counts.Next() {
		var key string
		var count int
		if err := counts.Scan(&key, &count); err != nil {
			return Page{}, err
		}
		page.Summary.Status[key] = count
		page.Summary.Total += count
	}
	return page, counts.Err()
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
