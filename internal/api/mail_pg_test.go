package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/hkjang/igame/internal/mail"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setTestUser writes the columns the mail directory reads.
func setTestUser(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, email, team, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE users SET email=$2,team=$3,status=$4 WHERE id=$1`, id, email, team, status); err != nil {
		t.Fatal(err)
	}
}

// The directory is the one query mail borrows from the users table. What it
// leaves out is as important as what it returns: a disabled account, an
// account without an address and an account that opted out are not written
// to, and nothing downstream needs to check again.
func TestMailDirectoryReturnsOnlyWhoMayBeWrittenTo(t *testing.T) {
	pool := migratedPool(t)
	s := &Server{DB: pool, Log: slog.Default()}
	ctx := context.Background()
	withAddress, disabled, blank, optedOut := insertTestUser(t, pool, "user"), insertTestUser(t, pool, "user"), insertTestUser(t, pool, "user"), insertTestUser(t, pool, "user")
	setTestUser(t, pool, withAddress, "kim@corp.example", "", "active")
	setTestUser(t, pool, disabled, "gone@corp.example", "", "disabled")
	setTestUser(t, pool, blank, "", "", "active")
	setTestUser(t, pool, optedOut, "quiet@corp.example", "", "active")
	if _, err := pool.Exec(ctx, `INSERT INTO user_preferences(user_id,value) VALUES($1,'{"mail_opt_out":true}')`, optedOut); err != nil {
		t.Fatal(err)
	}
	addresses, err := s.lookupEmails(ctx, []uuid.UUID{withAddress, disabled, blank, optedOut, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[withAddress] != "kim@corp.example" {
		t.Fatalf("directory = %v", addresses)
	}
}

func TestReviewersAreAdminsAndTheRequestersOwnManagers(t *testing.T) {
	pool := migratedPool(t)
	s := &Server{DB: pool, Log: slog.Default()}
	ctx := context.Background()
	admin, sameTeam, otherTeam, operator := insertTestUser(t, pool, "admin"), insertTestUser(t, pool, "manager"), insertTestUser(t, pool, "manager"), insertTestUser(t, pool, "operator")
	setTestUser(t, pool, admin, "a@corp.example", "", "active")
	setTestUser(t, pool, sameTeam, "m1@corp.example", "platform", "active")
	setTestUser(t, pool, otherTeam, "m2@corp.example", "games", "active")
	setTestUser(t, pool, operator, "o@corp.example", "", "active")
	has := func(ids []uuid.UUID, id uuid.UUID) bool {
		for _, candidate := range ids {
			if candidate == id {
				return true
			}
		}
		return false
	}
	reviewers, err := s.reviewersFor(ctx, "platform", false)
	if err != nil {
		t.Fatal(err)
	}
	if !has(reviewers, admin) || !has(reviewers, sameTeam) || has(reviewers, otherTeam) || has(reviewers, operator) {
		t.Fatalf("manager_required reviewers = %v", reviewers)
	}
	reviewers, err = s.reviewersFor(ctx, "platform", true)
	if err != nil {
		t.Fatal(err)
	}
	if !has(reviewers, operator) || has(reviewers, otherTeam) {
		t.Fatalf("reviewers with operators = %v", reviewers)
	}
	// A requester without a team has no manager to write to; the empty team
	// must not match every manager whose team is also empty.
	setTestUser(t, pool, otherTeam, "m2@corp.example", "", "active")
	reviewers, err = s.reviewersFor(ctx, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if has(reviewers, otherTeam) {
		t.Fatal("a manager with no team must not be matched by a requester with no team")
	}
}

func TestModerateScoreReportsTheOwnerAndThePreviousStanding(t *testing.T) {
	pool := migratedPool(t)
	s := &Server{DB: pool, Log: slog.Default()}
	ctx := context.Background()
	owner := insertTestUser(t, pool, "user")
	var gameID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM games ORDER BY created_at LIMIT 1`).Scan(&gameID); err != nil {
		t.Skip("no seeded game to score against:", err)
	}
	var sessionID, scoreID uuid.UUID
	hash := sha256.Sum256([]byte(uuid.NewString()))
	if err := pool.QueryRow(ctx, `INSERT INTO game_sessions(user_id,game_id,session_token_hash,status) VALUES($1,$2,$3,'finished') RETURNING id`, owner, gameID, hash[:]).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO scores(user_id,game_id,session_id,score) VALUES($1,$2,$3,4200) RETURNING id`, owner, gameID, sessionID).Scan(&scoreID); err != nil {
		t.Fatal(err)
	}
	first, err := s.moderateScore(ctx, scoreID, "excluded", false, "moderated_excluded")
	if err != nil {
		t.Fatal(err)
	}
	if first.owner != owner || first.score != 4200 || first.previous != "valid" || first.game == "" {
		t.Fatalf("first moderation = %+v", first)
	}
	second, err := s.moderateScore(ctx, scoreID, "excluded", false, "moderated_excluded")
	if err != nil {
		t.Fatal(err)
	}
	if second.previous != "excluded" {
		t.Fatalf("repeating the decision must report it as unchanged: %+v", second)
	}
	if _, err := s.moderateScore(ctx, uuid.New(), "excluded", false, ""); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown score: %v", err)
	}
}

// Every attempt leaves a row, sent or failed, and the listing carries both.
func TestMailDeliveriesRecordSuccessAndFailure(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()
	actor := insertTestUser(t, pool, "admin")
	recipient := insertTestUser(t, pool, "user")
	setTestUser(t, pool, recipient, "kim@corp.example", "", "active")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM mail_deliveries WHERE recipient IN ('kim@corp.example','ops@corp.example')`)
	})
	s := &Server{DB: pool, Log: slog.Default()}
	config := mail.Config{Enabled: true, SMTPHost: "relay.corp", FromAddress: "igame@corp.example"}.Normalized()
	service := mail.NewService(pool, func(context.Context) (mail.Config, error) { return config, nil }, s.lookupEmails, slog.Default())
	service.SetSender(func(_ context.Context, _ mail.Config, message mail.Message) error {
		if message.To == "ops@corp.example" {
			return errors.New("451 relay busy")
		}
		return nil
	})
	service.Notify(ctx, mail.ApprovalRequested("홍길동", "게임 등록", "Snake", "workflow_request", "1", "/reviews"), actor, []uuid.UUID{recipient})
	service.Wait()
	if err := service.SendNow(ctx, mail.TestMessage(), actor, "ops@corp.example"); err == nil {
		t.Fatal("the relay's refusal must reach the caller")
	}
	page, err := service.Deliveries(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	byRecipient := map[string]mail.Delivery{}
	for _, item := range page.Items {
		byRecipient[item.Recipient] = item
	}
	sent, failed := byRecipient["kim@corp.example"], byRecipient["ops@corp.example"]
	if sent.Status != "sent" || sent.Attempts != 1 || sent.Event != mail.EventApprovalRequested || sent.ResourceID != "1" || sent.ActorID == nil || *sent.ActorID != actor {
		t.Fatalf("sent record = %+v", sent)
	}
	if failed.Status != "failed" || failed.Attempts != 1 || failed.ErrorMessage != "451 relay busy" || failed.Event != mail.EventTest {
		t.Fatalf("failed record = %+v", failed)
	}
	if page.Summary.Status["sent"] < 1 || page.Summary.Status["failed"] < 1 {
		t.Fatalf("summary = %+v", page.Summary)
	}
	failedOnly, err := service.Deliveries(ctx, "failed", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range failedOnly.Items {
		if item.Status != "failed" {
			t.Fatalf("status filter leaked %+v", item)
		}
	}
}
