package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/hkjang/igame/internal/tracking"
	"github.com/jackc/pgx/v5"
)

// cspReportPath is where browsers post the requests the content security
// policy refused. It is unauthenticated because the browser sends the report
// without credentials, and it stores nothing but a bounded list of origins in
// memory. It is only named in the policy while tracking is on.
const cspReportPath = "/api/v1/tracking/csp-report"

// maxReportBytes keeps an unauthenticated endpoint from being used to push
// large bodies at the server.
const maxReportBytes = 8 * 1024

// maxProxyBodyBytes bounds what the same-origin proxy forwards to a Momento
// collector. A page view event is a few hundred bytes.
const maxProxyBodyBytes = 64 * 1024

const nonceKey contextKey = 2

// requestNonce is the value the policy header for this request allows inline
// scripts to carry. It is set only on page requests while tracking is on.
func requestNonce(r *http.Request) string {
	nonce, _ := r.Context().Value(nonceKey).(string)
	return nonce
}

// trackingConfig reads the tracking setting. A missing key is a deployment
// that never turned tracking on, and a read failure is treated the same way:
// the page must keep loading during a settings outage, and a page without a
// tracking snippet is the safe shape for it to load in.
func (s *Server) trackingConfig(ctx context.Context) tracking.Config {
	var config tracking.Config
	if err := s.setting(ctx, "tracking", &config); err != nil {
		return tracking.Config{}
	}
	return config.Normalized()
}

// pagePath reports whether a path serves a document a person looks at. The
// API, MCP, health and proxy paths never carry the snippet, and their policy
// is narrowed instead of widened.
func pagePath(path string) bool {
	if strings.HasPrefix(path, "/api/") || path == "/mcp" || strings.HasPrefix(path, "/mcp/") ||
		path == "/healthz" || path == "/readyz" || strings.HasPrefix(path, "/.well-known/") {
		return false
	}
	return path != tracking.ProxyPrefix && !strings.HasPrefix(path, tracking.ProxyPrefix+"/")
}

// pagePolicy builds the policy for a document. The strict base stays as it
// was; while tracking is active on this page the nonce, the sources the
// snippet needs and the report address are added — and nothing else, so
// turning tracking off returns the policy to exactly its old shape.
func pagePolicy(config tracking.Config, path, nonce string, frames, connect []string) string {
	scripts := []string{"'self'"}
	images := []string{"'self'", "data:", "blob:"}
	active := config.Active(path) && nonce != ""
	if active {
		extraScripts, extraConnects, extraImages := config.PolicySources()
		scripts = append(scripts, "'nonce-"+nonce+"'")
		scripts = append(scripts, extraScripts...)
		connect = append(append([]string{}, connect...), extraConnects...)
		images = append(images, extraImages...)
	}
	policy := "default-src 'self'; script-src " + strings.Join(scripts, " ") +
		"; style-src 'self' 'unsafe-inline'; img-src " + strings.Join(images, " ") +
		"; font-src 'self' data:; connect-src " + strings.Join(connect, " ") +
		"; frame-src " + strings.Join(frames, " ") +
		"; object-src 'none'; base-uri 'self'; frame-ancestors 'self'; form-action 'self'"
	if active {
		// Ask the browser to say what it refused. That report is what turns a
		// console error into a one-click fix on the settings screen.
		policy += "; report-uri " + cspReportPath
	}
	return policy
}

// nonPagePolicy is the policy for responses nobody renders: JSON, streams,
// health answers and the proxied collector. Nothing may load from them.
const nonPagePolicy = "default-src 'none'; frame-ancestors 'self'"

// injectSnippet places the markup just before the closing tag its placement
// names, falling back to the end of the document when the tag is missing.
func injectSnippet(page []byte, snippet, placement string) []byte {
	marker := "</head>"
	if placement == "body" {
		marker = "</body>"
	}
	text := string(page)
	index := strings.LastIndex(strings.ToLower(text), marker)
	if index < 0 {
		return []byte(text + "\n" + snippet + "\n")
	}
	return []byte(text[:index] + snippet + "\n" + text[index:])
}

// rewriteIndex adds the tracking snippet to the SPA shell for one request. The
// nonce it carries is the one securityHeaders put in this request's policy, so
// the header and the markup always agree.
func (s *Server) rewriteIndex(r *http.Request, index []byte) []byte {
	nonce := requestNonce(r)
	if nonce == "" {
		return index
	}
	config := s.trackingConfig(r.Context())
	if !config.Active(r.URL.Path) {
		return index
	}
	snippet := config.Snippet(nonce)
	if snippet == "" {
		return index
	}
	return injectSnippet(index, snippet, config.Placement)
}

// proxyMomento forwards /momento/* to the configured collector so the snippet
// can load the tracker and post events on this origin. It answers 404 unless
// tracking is on, the provider is Momento and the proxy is chosen, which keeps
// the service from being an open relay to anywhere.
func (s *Server) proxyMomento(w http.ResponseWriter, r *http.Request) {
	target := s.trackingConfig(r.Context()).ProxyTarget()
	if target == "" {
		http.NotFound(w, r)
		return
	}
	upstream, err := url.Parse(target)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var transport http.RoundTripper
	if s.HTTP != nil {
		transport = s.HTTP.Transport
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Path = strings.TrimPrefix(pr.Out.URL.Path, tracking.ProxyPrefix)
			pr.Out.URL.RawPath = ""
			pr.SetURL(upstream)
			// The collector is a different service: it gets neither this
			// service's session cookie nor a bearer key.
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("Authorization")
			pr.SetXForwarded()
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.logRequestError(r, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProxyBodyBytes)
	proxy.ServeHTTP(w, r)
}

type cspReport struct {
	Report struct {
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		DocumentURI        string `json:"document-uri"`
	} `json:"csp-report"`
}

// receiveCSPReport records what a browser refused to load. Reports are always
// answered with 204: a page that cannot be tracked must not also see an error.
func (s *Server) receiveCSPReport(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.violations == nil {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxReportBytes))
	if err != nil || len(body) == 0 {
		return
	}
	var report cspReport
	if json.Unmarshal(body, &report) != nil {
		return
	}
	directive := report.Report.EffectiveDirective
	if directive == "" {
		directive = report.Report.ViolatedDirective
	}
	s.violations.Record(report.Report.BlockedURI, directive, report.Report.DocumentURI)
}

// listTrackingViolations shows the administrator which addresses the policy
// is blocking, so a snippet can be fixed without reading a browser console.
func (s *Server) listTrackingViolations(w http.ResponseWriter, r *http.Request) {
	items := []tracking.Violation{}
	if s.violations != nil {
		items = s.violations.List(s.trackingConfig(r.Context()))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// clearTrackingViolations forgets the recorded reports, which is how an
// administrator checks whether a change actually fixed the snippet.
func (s *Server) clearTrackingViolations(w http.ResponseWriter, r *http.Request) {
	if s.violations != nil {
		s.violations.Forget()
	}
	w.WriteHeader(http.StatusNoContent)
}

// allowTrackingHost adds one blocked origin to the tracking allow list: the
// one-click fix for the reports listed above. It goes through the same write
// as the settings screen so the change is audited like any other.
func (s *Server) allowTrackingHost(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Origin string `json:"origin"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	origin, ok := tracking.Origin(input.Origin)
	if !ok {
		writeError(w, 400, "invalid_origin", "origin must be an http(s) origin")
		return
	}
	var current tracking.Config
	if err := s.setting(r.Context(), "tracking", &current); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.serverError(w, r, 503, "tracking_setting_unavailable", "the stored tracking setting could not be read", err)
		return
	}
	current.AllowedHosts = tracking.AddAllowedHost(current.AllowedHosts, origin)
	value, err := json.Marshal(current)
	if err != nil {
		s.serverError(w, r, 500, "internal_error", "internal server error", err)
		return
	}
	p, _ := principalFrom(r)
	var previous json.RawMessage
	err = s.DB.QueryRow(r.Context(), `WITH before AS (SELECT value FROM system_settings WHERE key=$1)
		INSERT INTO system_settings(key,value,updated_by) VALUES($1,$2,$3)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_by=excluded.updated_by,updated_at=now()
		RETURNING COALESCE((SELECT value FROM before),'{}'::jsonb)`, "tracking", value, p.UserID).Scan(&previous)
	if err != nil {
		s.dbError(w, r, err)
		return
	}
	s.invalidateSetting("tracking")
	s.audit(r, "setting.update", "setting", "tracking", auditSettingChange(previous, value))
	writeJSON(w, 200, map[string]any{"key": "tracking", "value": json.RawMessage(value)})
}
