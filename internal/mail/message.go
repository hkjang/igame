package mail

import (
	"fmt"
	"strings"
)

// Notification is the content of one event mail before recipients are
// resolved. Link is a path on this service; Render makes it absolute with
// the configured base URL, and drops it when there is none.
type Notification struct {
	Event        string
	Subject      string
	Lines        []string
	Link         string
	ResourceType string
	ResourceID   string
}

// Render turns a notification into the message body, appending the link and
// a footer that says why the mail arrived.
func (n Notification) Render(config Config) string {
	lines := append([]string{}, n.Lines...)
	if link := n.absoluteLink(config); link != "" {
		lines = append(lines, "", "바로 열기: "+link)
	}
	lines = append(lines, "", "—", "이 메일은 igame 알림 설정에 따라 자동으로 발송되었습니다. 관리자가 관리 화면 > 메일 알림에서 종류별로 끌 수 있습니다.")
	return strings.Join(lines, "\n")
}

func (n Notification) absoluteLink(config Config) string {
	if n.Link == "" {
		return ""
	}
	if strings.HasPrefix(n.Link, "http://") || strings.HasPrefix(n.Link, "https://") {
		return n.Link
	}
	if config.BaseURL == "" {
		return ""
	}
	return config.BaseURL + "/" + strings.TrimLeft(n.Link, "/")
}

// ApprovalRequested tells a reviewer that somebody is waiting on them. It is
// the one mail that unblocks another person, which is why it comes first.
func ApprovalRequested(requester, kind, title, resourceType, resourceID, link string) Notification {
	return Notification{
		Event:        EventApprovalRequested,
		Subject:      fmt.Sprintf("[igame] 승인 요청: %s — %s", kind, title),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Link:         link,
		Lines: []string{
			fmt.Sprintf("%s 님이 %s '%s'의 검토를 요청했습니다.", requester, kind, title),
			"검토·승인 화면에서 승인하거나 반려 사유를 적어 돌려보낼 수 있습니다.",
		},
	}
}

// ApprovalDecided tells the requester what the reviewer decided, with the
// comment when there is one — a rejection always has one.
func ApprovalDecided(reviewer, kind, title, decision, comment, resourceType, resourceID, link string) Notification {
	verdict := "반려되었습니다"
	if decision == "approved" || decision == "applied" {
		verdict = "승인되었습니다"
	}
	lines := []string{fmt.Sprintf("%s 님이 검토한 %s '%s'이(가) %s.", reviewer, kind, title, verdict)}
	if strings.TrimSpace(comment) != "" {
		lines = append(lines, "", quote(comment))
	}
	return Notification{
		Event:        EventApprovalDecided,
		Subject:      fmt.Sprintf("[igame] %s — %s '%s'", verdict, kind, title),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Link:         link,
		Lines:        lines,
	}
}

// RankingModerated tells a player that an operator changed the standing of
// one of their scores. Losing a place on the board without a word is the
// kind of thing people refresh the page over.
func RankingModerated(game string, score int64, status, reason, scoreID string) Notification {
	what := map[string]string{"valid": "다시 유효 처리되었습니다", "flagged": "검토 대상으로 표시되었습니다", "excluded": "랭킹에서 제외되었습니다"}[status]
	if what == "" {
		what = "상태가 바뀌었습니다"
	}
	lines := []string{fmt.Sprintf("'%s'에서 기록한 %d점이 운영자에 의해 %s.", game, score, what)}
	if reason != "" && !strings.HasPrefix(reason, "moderated_") {
		lines = append(lines, "", "사유: "+reason)
	}
	return Notification{
		Event:        EventRankingModerated,
		Subject:      fmt.Sprintf("[igame] %s 점수가 %s", game, what),
		ResourceType: "score",
		ResourceID:   scoreID,
		Link:         "/rankings",
		Lines:        lines,
	}
}

// TestMessage proves the relay works from the settings screen.
func TestMessage() Notification {
	return Notification{
		Event:   EventTest,
		Subject: "[igame] SMTP 발송 테스트",
		Lines:   []string{"igame 관리 화면에서 보낸 테스트 메일입니다.", "이 메일을 받았다면 SMTP 설정이 정상입니다."},
	}
}

func quote(body string) string {
	trimmed := strings.TrimSpace(body)
	if runes := []rune(trimmed); len(runes) > 500 {
		trimmed = string(runes[:500]) + "…"
	}
	lines := strings.Split(trimmed, "\n")
	for index, line := range lines {
		lines[index] = "> " + line
	}
	return strings.Join(lines, "\n")
}
