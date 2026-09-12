package tracking

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MaxViolations bounds the recorder. A blocked request repeats on every page
// view, so what matters is which origins are blocked, not how many times — a
// small buffer of distinct origins is enough to fix a snippet.
const MaxViolations = 100

// Violation is one origin the content security policy refused, kept with the
// directive that refused it so the settings screen can say what to allow.
type Violation struct {
	Origin    string    `json:"origin"`
	Directive string    `json:"directive"`
	Page      string    `json:"page"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// Allowed marks an origin the current configuration already permits, so a
	// report that arrived before the fix does not keep nagging.
	Allowed bool `json:"allowed"`
}

// Recorder collects the policy violations browsers report. It is deliberately
// in memory: the reports are a live troubleshooting aid for the person pasting
// a snippet, not an audit record, and keeping them out of the database means a
// browser can report freely without growing storage.
type Recorder struct {
	mu         sync.Mutex
	violations map[string]*Violation
	now        func() time.Time
}

func NewRecorder(now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{violations: map[string]*Violation{}, now: now}
}

// Record notes one blocked request. Anything that is not an http(s) origin —
// a browser extension, a data: URL, the "inline" and "eval" markers — is
// dropped: allowing it is neither possible nor wanted.
func (r *Recorder) Record(blockedURI, directive, page string) {
	origin, ok := Origin(blockedURI)
	if !ok {
		return
	}
	directive = strings.TrimSpace(strings.ToLower(directive))
	if index := strings.IndexByte(directive, ' '); index > 0 {
		directive = directive[:index]
	}
	if directive == "" {
		directive = "connect-src"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := directive + " " + origin
	moment := r.now()
	if existing, found := r.violations[key]; found {
		existing.Count++
		existing.LastSeen = moment
		existing.Page = page
		return
	}
	if len(r.violations) >= MaxViolations {
		r.evictOldest()
	}
	r.violations[key] = &Violation{Origin: origin, Directive: directive, Page: page, Count: 1, FirstSeen: moment, LastSeen: moment}
}

func (r *Recorder) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for key, violation := range r.violations {
		if oldestKey == "" || violation.LastSeen.Before(oldest) {
			oldestKey, oldest = key, violation.LastSeen
		}
	}
	delete(r.violations, oldestKey)
}

// List returns the blocked origins, most recent first, marking the ones the
// configuration already allows.
func (r *Recorder) List(config Config) []Violation {
	allowed := map[string]struct{}{}
	scripts, connects, images := config.PolicySources()
	for _, group := range [][]string{scripts, connects, images} {
		for _, origin := range group {
			allowed[origin] = struct{}{}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Violation, 0, len(r.violations))
	for _, violation := range r.violations {
		copied := *violation
		_, known := allowed[copied.Origin]
		copied.Allowed = known || matchesWildcard(copied.Origin, allowed)
		items = append(items, copied)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].LastSeen.Equal(items[j].LastSeen) {
			return items[i].Origin < items[j].Origin
		}
		return items[i].LastSeen.After(items[j].LastSeen)
	})
	return items
}

// Forget drops the recorded reports, which is what an administrator does after
// fixing a snippet to see whether anything is still blocked.
func (r *Recorder) Forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.violations = map[string]*Violation{}
}

// matchesWildcard covers policy entries such as https://*.google-analytics.com.
func matchesWildcard(origin string, allowed map[string]struct{}) bool {
	for pattern := range allowed {
		scheme, host, found := strings.Cut(pattern, "://*.")
		if !found {
			continue
		}
		if strings.HasPrefix(origin, scheme+"://") && strings.HasSuffix(origin, "."+host) {
			return true
		}
	}
	return false
}
