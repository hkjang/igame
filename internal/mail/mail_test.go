package mail

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRelay speaks just enough SMTP to accept one message, with or without
// an AUTH offer, and hands the dialogue back to the test.
type fakeRelay struct {
	listener net.Listener
	auth     bool
	mu       sync.Mutex
	commands []string
	data     string
}

func startRelay(t *testing.T, auth bool) *fakeRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay := &fakeRelay{listener: listener, auth: auth}
	go relay.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return relay
}

func (f *fakeRelay) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeRelay) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 relay.test ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.mu.Lock()
		f.commands = append(f.commands, line)
		f.mu.Unlock()
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			write("250-relay.test")
			if f.auth {
				write("250-AUTH PLAIN LOGIN")
			}
			write("250 8BITMIME")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			write("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			write("250 ok")
		case upper == "DATA":
			write("354 go ahead")
			var body strings.Builder
			for {
				part, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if part == ".\r\n" {
					break
				}
				body.WriteString(part)
			}
			f.mu.Lock()
			f.data = body.String()
			f.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func (f *fakeRelay) config() Config {
	host, port, _ := net.SplitHostPort(f.listener.Addr().String())
	var number int
	for _, digit := range port {
		number = number*10 + int(digit-'0')
	}
	return Config{Enabled: true, SMTPHost: host, SMTPPort: number, Security: "none", FromAddress: "igame@example.test", FromName: "igame 알림", Timeout: 5}
}

func (f *fakeRelay) saw(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, command := range f.commands {
		if strings.HasPrefix(strings.ToUpper(command), strings.ToUpper(prefix)) {
			return true
		}
	}
	return false
}

func TestDeliverSendsWithoutCredentialsOnAPlainRelay(t *testing.T) {
	relay := startRelay(t, false)
	err := Deliver(context.Background(), relay.config(), Message{To: "kim@example.test", Subject: "승인 요청", Body: "첫 줄\n.둘째 줄은 점으로 시작\n"})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if relay.saw("AUTH") {
		t.Fatal("a relay that offers no AUTH must not be sent credentials")
	}
	if !relay.saw("MAIL FROM:<igame@example.test>") || !relay.saw("RCPT TO:<kim@example.test>") {
		t.Fatalf("envelope missing: %v", relay.commands)
	}
	relay.mu.Lock()
	data := relay.data
	relay.mu.Unlock()
	if !strings.Contains(data, "Subject: =?utf-8?q?") {
		t.Fatalf("subject must be encoded for old relays: %q", data)
	}
	if !strings.Contains(data, "\r\n..둘째") {
		t.Fatalf("a line starting with a dot must be dot-stuffed exactly once: %q", data)
	}
	if !strings.Contains(data, "Auto-Submitted: auto-generated") {
		t.Fatalf("auto-submitted header missing: %q", data)
	}
}

func TestDeliverAuthenticatesOnlyWhenAsked(t *testing.T) {
	relay := startRelay(t, true)
	config := relay.config()
	if err := Deliver(context.Background(), config, Message{To: "kim@example.test", Subject: "x", Body: "y"}); err != nil {
		t.Fatalf("deliver without username: %v", err)
	}
	if relay.saw("AUTH") {
		t.Fatal("an empty username must skip AUTH even when the relay offers it")
	}
	config.Username, config.Password = "igame", "secret"
	if err := Deliver(context.Background(), config, Message{To: "kim@example.test", Subject: "x", Body: "y"}); err != nil {
		t.Fatalf("deliver with username: %v", err)
	}
	if !relay.saw("AUTH PLAIN") {
		t.Fatalf("expected PLAIN auth: %v", relay.commands)
	}
}

func TestDeliverRefusesToSendWhenTheSettingIsIncomplete(t *testing.T) {
	config := Config{Enabled: true, SMTPPort: 25, Security: "auto", Timeout: 5}
	err := Deliver(context.Background(), config, Message{To: "kim@example.test"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid without a host, got %v", err)
	}
}

func TestValidateNormalizesAndChecks(t *testing.T) {
	config := Config{}.Normalized()
	if config.SMTPPort != 25 || config.Security != "auto" || config.Timeout != 10 {
		t.Fatalf("defaults: %+v", config)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("the default (disabled) setting must be valid: %v", err)
	}
	if err := (Config{SMTPPort: 465}).Normalized().Validate(); err != nil {
		t.Fatal(err)
	}
	if (Config{SMTPPort: 465}).Normalized().Security != "tls" {
		t.Fatal("port 465 implies implicit TLS")
	}
	if err := (Config{Enabled: true}).Normalized().Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("enabled without host must fail: %v", err)
	}
	if err := (Config{Security: "ssl"}).Normalized().Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown security must fail: %v", err)
	}
	if err := (Config{FromAddress: "not-an-address"}).Normalized().Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad from address must fail: %v", err)
	}
	if err := (Config{BaseURL: "igame.local"}).Normalized().Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("relative base url must fail: %v", err)
	}
	off := false
	config = Config{NotifyRankingModerated: &off}
	if config.Allows(EventRankingModerated) || !config.Allows(EventApprovalRequested) || !config.Allows(EventTest) {
		t.Fatal("only the switched-off event may be silenced")
	}
	redacted, configured := (Config{Password: "hunter2"}).Redacted()
	if redacted.Password != "" || !configured {
		t.Fatal("redacted config must drop the password and say one is stored")
	}
}

// capture is a Service with the transport replaced, so a test sees what would
// have gone to the relay.
type capture struct {
	mu       sync.Mutex
	messages []Message
	fail     error
}

func (c *capture) send(_ context.Context, _ Config, message Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, message)
	return c.fail
}

func newTestService(config Config, directory map[uuid.UUID]string) (*Service, *capture) {
	sink := &capture{}
	service := NewService(nil, func(context.Context) (Config, error) { return config, nil },
		func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
			out := map[uuid.UUID]string{}
			for _, id := range ids {
				if address, ok := directory[id]; ok {
					out[id] = address
				}
			}
			return out, nil
		}, nil)
	service.SetSender(sink.send)
	return service, sink
}

func TestNotifySkipsTheActorAndDuplicateAddresses(t *testing.T) {
	actor, manager, admin, silent := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	directory := map[uuid.UUID]string{actor: "actor@example.test", manager: "lead@example.test", admin: "Lead@example.test"}
	service, sink := newTestService(Config{Enabled: true, SMTPHost: "relay", FromAddress: "igame@example.test"}, directory)
	service.Notify(context.Background(), ApprovalRequested("홍길동", "게임", "Snake", "workflow_request", "1", "/admin/approvals"), actor, []uuid.UUID{actor, manager, admin, silent})
	service.Wait()
	if len(sink.messages) != 1 || sink.messages[0].To != "lead@example.test" {
		t.Fatalf("expected one mail to the manager, got %+v", sink.messages)
	}
	if !strings.Contains(sink.messages[0].Body, "홍길동") {
		t.Fatalf("body: %q", sink.messages[0].Body)
	}
}

func TestNotifySendsNothingWhenOffOrSwitchedOff(t *testing.T) {
	recipient := uuid.New()
	directory := map[uuid.UUID]string{recipient: "kim@example.test"}
	service, sink := newTestService(Config{Enabled: false, SMTPHost: "relay", FromAddress: "igame@example.test"}, directory)
	service.Notify(context.Background(), TestMessage(), uuid.Nil, []uuid.UUID{recipient})
	service.Wait()
	if len(sink.messages) != 0 {
		t.Fatal("disabled mail must send nothing")
	}
	off := false
	service, sink = newTestService(Config{Enabled: true, SMTPHost: "relay", FromAddress: "igame@example.test", NotifyApprovalDecided: &off}, directory)
	service.Notify(context.Background(), ApprovalDecided("팀장", "게임", "Snake", "rejected", "설명이 부족합니다", "workflow_request", "1", ""), uuid.Nil, []uuid.UUID{recipient})
	service.Notify(context.Background(), RankingModerated("Snake", 1200, "excluded", "moderated_excluded", "2"), uuid.Nil, []uuid.UUID{recipient})
	service.Wait()
	if len(sink.messages) != 1 || sink.messages[0].Subject != "[igame] Snake 점수가 랭킹에서 제외되었습니다" {
		t.Fatalf("only the still-enabled event may go out: %+v", sink.messages)
	}
}

func TestNotifyDoesNotBlockWhenTheRelayIsDown(t *testing.T) {
	recipient := uuid.New()
	directory := map[uuid.UUID]string{recipient: "kim@example.test"}
	service, sink := newTestService(Config{Enabled: true, SMTPHost: "relay", FromAddress: "igame@example.test", Timeout: 1}, directory)
	release := make(chan struct{})
	service.SetSender(func(ctx context.Context, _ Config, message Message) error {
		<-release
		return sink.send(ctx, Config{}, message)
	})
	done := make(chan struct{})
	go func() {
		service.Notify(context.Background(), TestMessage(), uuid.Nil, []uuid.UUID{recipient})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify must return before the relay answers")
	}
	sink.fail = errors.New("connection refused")
	close(release)
	service.Wait()
	if len(sink.messages) != 2 {
		t.Fatalf("a failed send is retried once, got %d attempts", len(sink.messages))
	}
}

func TestSendNowReportsTheOutcomeAndRefusesWhenDisabled(t *testing.T) {
	service, sink := newTestService(Config{Enabled: false}, nil)
	if err := service.SendNow(context.Background(), TestMessage(), uuid.Nil, "kim@example.test"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
	service, sink = newTestService(Config{Enabled: true, SMTPHost: "relay", FromAddress: "igame@example.test", BaseURL: "https://igame.local"}, nil)
	sink.fail = errors.New("550 relay refused")
	if err := service.SendNow(context.Background(), TestMessage(), uuid.Nil, "kim@example.test"); err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("the relay's answer must reach the caller: %v", err)
	}
	if len(sink.messages) != 1 {
		t.Fatal("the test mail is sent once, without retry")
	}
}

func TestRenderAddsTheLinkOnlyWithABaseURL(t *testing.T) {
	notification := ApprovalRequested("홍길동", "RealmGuard 콘텐츠", "v3", "realmguard_content_version", "1", "/admin/realmguard")
	if strings.Contains(notification.Render(Config{}), "바로 열기") {
		t.Fatal("no base URL, no link")
	}
	if !strings.Contains(notification.Render(Config{BaseURL: "https://igame.local"}), "바로 열기: https://igame.local/admin/realmguard") {
		t.Fatal("link must be made absolute with the base URL")
	}
}
