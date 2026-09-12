// Package tracking injects a visitor tracking snippet into the portal shell.
//
// The content security policy igame ships allows scripts from its own origin
// only, so a tracking snippet cannot simply be pasted into index.html. This
// package produces both halves of the answer: the markup to inject and the
// policy sources it needs, with a per-request nonce so the inline part of a
// snippet runs without loosening the policy for everything else.
package tracking

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

const (
	ProviderNone    = "none"
	ProviderMomento = "momento"
	ProviderGA4     = "ga4"
	ProviderGTM     = "gtm"
	ProviderMatomo  = "matomo"
	ProviderCustom  = "custom"

	// MaxSnippetBytes bounds a pasted snippet. A loader is a few hundred bytes;
	// anything larger is not a loader.
	MaxSnippetBytes = 8 * 1024

	// ProxyPrefix is the same-origin path the service forwards to a Momento
	// collector. A snippet that points at it needs no external origin in the
	// policy at all, which is why it is the default for Momento.
	ProxyPrefix = "/momento"
)

// Providers lists the accepted provider values. Momento comes first: it is the
// self-hosted collector, and the only choice that keeps the data inside.
var Providers = []string{ProviderNone, ProviderMomento, ProviderGA4, ProviderGTM, ProviderMatomo, ProviderCustom}

// Config is the stored "tracking" setting. Everything is optional and the zero
// value tracks nothing, so a deployment that never saved the setting behaves
// exactly as it did before the setting existed.
type Config struct {
	Enabled            bool   `json:"enabled"`
	Provider           string `json:"provider"`
	MomentoURL         string `json:"momento_url,omitempty"`
	MomentoSiteID      string `json:"momento_site_id,omitempty"`
	MomentoEnvironment string `json:"momento_environment,omitempty"`
	// MomentoProxy is a pointer so that an absent field means "proxied": the
	// same-origin path is the default, and a stored setting that predates the
	// field keeps it.
	MomentoProxy  *bool  `json:"momento_proxy,omitempty"`
	MeasurementID string `json:"measurement_id,omitempty"`
	MatomoURL     string `json:"matomo_url,omitempty"`
	MatomoSiteID  string `json:"matomo_site_id,omitempty"`
	CustomSnippet string `json:"custom_snippet,omitempty"`
	AllowedHosts  string `json:"allowed_hosts,omitempty"`
	IncludeAdmin  bool   `json:"include_admin"`
	Placement     string `json:"placement,omitempty"`
}

// Normalized fills in the defaults a stored value may omit and trims what an
// administrator typed.
func (c Config) Normalized() Config {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = ProviderNone
	}
	c.Placement = strings.ToLower(strings.TrimSpace(c.Placement))
	if c.Placement != "body" {
		c.Placement = "head"
	}
	c.MomentoURL = strings.TrimRight(strings.TrimSpace(c.MomentoURL), "/")
	c.MomentoSiteID = strings.TrimSpace(c.MomentoSiteID)
	c.MomentoEnvironment = strings.TrimSpace(c.MomentoEnvironment)
	if c.MomentoEnvironment == "" {
		c.MomentoEnvironment = "prd"
	}
	c.MeasurementID = strings.TrimSpace(c.MeasurementID)
	c.MatomoURL = strings.TrimRight(strings.TrimSpace(c.MatomoURL), "/")
	c.MatomoSiteID = strings.TrimSpace(c.MatomoSiteID)
	c.CustomSnippet = strings.TrimSpace(c.CustomSnippet)
	return c
}

// MomentoProxied reports whether the Momento snippet goes through the
// same-origin proxy. It does unless an administrator switched it off.
func (c Config) MomentoProxied() bool {
	return c.MomentoProxy == nil || *c.MomentoProxy
}

// ProxyTarget is the collector the same-origin proxy forwards to, or "" when
// nothing should be forwarded: tracking off, another provider, or the proxy
// switched off. The proxy answers 404 in every one of those cases.
func (c Config) ProxyTarget() string {
	if !c.Enabled || c.Provider != ProviderMomento || !c.MomentoProxied() {
		return ""
	}
	if _, ok := Origin(c.MomentoURL); !ok {
		return ""
	}
	return c.MomentoURL
}

// Active reports whether a page at path should carry the snippet.
// Administrative screens are excluded unless asked for, because console
// traffic is rarely the visitor data anybody wants.
func (c Config) Active(path string) bool {
	if !c.Enabled || c.Provider == ProviderNone {
		return false
	}
	if !c.IncludeAdmin && (path == "/admin" || strings.HasPrefix(path, "/admin/")) {
		return false
	}
	return c.Snippet("") != ""
}

// Validate reports what is missing for the chosen provider, in the words the
// settings screen shows.
func (c Config) Validate() error {
	if len(c.CustomSnippet) > MaxSnippetBytes {
		return fmt.Errorf("custom_snippet must not exceed %d bytes", MaxSnippetBytes)
	}
	for _, host := range splitHosts(c.AllowedHosts) {
		if _, ok := Origin(host); !ok {
			return fmt.Errorf("allowed_hosts entry %q must be an http(s) origin", host)
		}
	}
	switch c.Provider {
	case ProviderNone:
	case ProviderMomento:
		if c.MomentoURL == "" || c.MomentoSiteID == "" {
			return fmt.Errorf("momento_url and momento_site_id are required")
		}
		if _, ok := Origin(c.MomentoURL); !ok {
			return fmt.Errorf("momento_url must be an absolute http(s) URL")
		}
	case ProviderGA4, ProviderGTM:
		if c.MeasurementID == "" {
			return fmt.Errorf("measurement_id is required")
		}
	case ProviderMatomo:
		if c.MatomoURL == "" || c.MatomoSiteID == "" {
			return fmt.Errorf("matomo_url and matomo_site_id are required")
		}
		if _, ok := Origin(c.MatomoURL); !ok {
			return fmt.Errorf("matomo_url must be an absolute http(s) URL")
		}
	case ProviderCustom:
		if c.CustomSnippet == "" {
			return fmt.Errorf("custom_snippet is empty")
		}
	default:
		return fmt.Errorf("provider must be one of %s", strings.Join(Providers, ", "))
	}
	return nil
}

// Snippet renders the markup to inject. The nonce goes on every script tag so
// the policy can stay strict.
func (c Config) Snippet(nonce string) string {
	switch c.Provider {
	case ProviderMomento:
		if c.MomentoSiteID == "" {
			return ""
		}
		attrs := fmt.Sprintf(` data-site-id="%s" data-environment="%s" data-contract-version="1"`,
			html.EscapeString(c.MomentoSiteID), html.EscapeString(c.MomentoEnvironment))
		if c.MomentoProxied() {
			// The tracker and its endpoint both live under this origin, so the
			// policy needs nothing beyond 'self' and the nonce.
			return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js"%s data-endpoint="%s"></script>`, ProxyPrefix, attrs, ProxyPrefix), nonce)
		}
		if c.MomentoURL == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js"%s></script>`, html.EscapeString(c.MomentoURL), attrs), nonce)
	case ProviderGA4:
		if c.MeasurementID == "" {
			return ""
		}
		id := html.EscapeString(c.MeasurementID)
		return withNonce(fmt.Sprintf(`<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','%s');</script>`, id, jsString(c.MeasurementID)), nonce)
	case ProviderGTM:
		if c.MeasurementID == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','%s');</script>`, jsString(c.MeasurementID)), nonce)
	case ProviderMatomo:
		if c.MatomoURL == "" || c.MatomoSiteID == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);(function(){var u="%s/";_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId','%s']);var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();</script>`, jsString(c.MatomoURL), jsString(c.MatomoSiteID)), nonce)
	case ProviderCustom:
		return withNonce(c.CustomSnippet, nonce)
	}
	return ""
}

// jsString keeps a configured value inside the JavaScript string literal it is
// written into: quotes, backslashes and the characters that would end a script
// element are escaped.
func jsString(value string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, `"`, `\"`, "<", `\x3c`, ">", `\x3e`, "\n", `\n`, "\r", `\r`).Replace(value)
}

// withNonce adds the nonce to every script tag that does not already carry
// one, which is what lets a pasted snippet run under a strict policy unchanged.
func withNonce(snippet, nonce string) string {
	if nonce == "" || snippet == "" {
		return snippet
	}
	var out strings.Builder
	remaining := snippet
	for {
		index := strings.Index(strings.ToLower(remaining), "<script")
		if index < 0 {
			out.WriteString(remaining)
			return out.String()
		}
		end := index + len("<script")
		out.WriteString(remaining[:end])
		tag := remaining[end:]
		if closing := strings.Index(tag, ">"); closing >= 0 {
			tag = tag[:closing]
		}
		if !strings.Contains(strings.ToLower(tag), "nonce=") {
			out.WriteString(` nonce="` + html.EscapeString(nonce) + `"`)
		}
		remaining = remaining[end:]
	}
}

// PolicySources lists the extra origins the snippet needs, in the order the
// policy directives want them: scripts, connections, images. A Momento setup
// behind the proxy needs none.
func (c Config) PolicySources() (scripts, connects, images []string) {
	add := func(origin string) {
		scripts = append(scripts, origin)
		connects = append(connects, origin)
		images = append(images, origin)
	}
	switch c.Provider {
	case ProviderMomento:
		if !c.MomentoProxied() {
			if origin, ok := Origin(c.MomentoURL); ok {
				add(origin)
			}
		}
	case ProviderGA4, ProviderGTM:
		scripts = append(scripts, "https://www.googletagmanager.com")
		connects = append(connects, "https://www.google-analytics.com", "https://analytics.google.com", "https://*.google-analytics.com")
		images = append(images, "https://www.google-analytics.com", "https://www.googletagmanager.com")
	case ProviderMatomo:
		if origin, ok := Origin(c.MatomoURL); ok {
			add(origin)
		}
	case ProviderCustom:
		// A pasted snippet names the addresses it loads and reports to, so those
		// origins are allowed without anybody reading a policy error first.
		for _, origin := range SnippetOrigins(c.CustomSnippet) {
			add(origin)
		}
	}
	for _, host := range splitHosts(c.AllowedHosts) {
		if origin, ok := Origin(host); ok {
			add(origin)
		}
	}
	return dedupe(scripts), dedupe(connects), dedupe(images)
}

// SnippetOrigins lists every http(s) origin written into a snippet: the script
// it loads, the endpoint it posts to, the pixel it requests. A tracker almost
// always writes its own address somewhere in its loader.
func SnippetOrigins(snippet string) []string {
	var origins []string
	lower := strings.ToLower(snippet)
	for index := 0; index < len(snippet); {
		start := strings.Index(lower[index:], "http")
		if start < 0 {
			break
		}
		start += index
		end := start
		for end < len(snippet) && !isURLBoundary(snippet[end]) {
			end++
		}
		index = end
		if origin, ok := Origin(snippet[start:end]); ok {
			origins = append(origins, origin)
		}
	}
	return dedupe(origins)
}

// isURLBoundary reports the characters that cannot appear in a URL written
// inside HTML or JavaScript, which is where each address ends.
func isURLBoundary(letter byte) bool {
	switch letter {
	case '"', '\'', '`', '<', '>', ' ', '\t', '\n', '\r', ')', ',', ';', '\\', '+':
		return true
	}
	return false
}

// Origin reduces an address to the scheme://host[:port] form a policy source
// takes. Only http(s) qualifies; a wildcard host such as *.example.com is kept
// because the policy language understands it. The result contains no character
// that could end a directive, so it is safe to write into the header.
func Origin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	if strings.ContainsAny(u.Host, " ;,'\"") {
		return "", false
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), true
}

// splitHosts reads the comma, space or newline separated allow list.
func splitHosts(list string) []string {
	return strings.FieldsFunc(list, func(letter rune) bool {
		return letter == ',' || letter == ' ' || letter == '\n' || letter == '\r' || letter == '\t'
	})
}

// AddAllowedHost appends an origin to the allow list, leaving the existing
// entries and their order alone.
func AddAllowedHost(existing, origin string) string {
	origin, ok := Origin(origin)
	if !ok {
		return existing
	}
	for _, host := range splitHosts(existing) {
		if known, _ := Origin(host); known == origin {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return origin
	}
	return strings.TrimSpace(existing) + ", " + origin
}

func dedupe(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := items[:0]
	for _, item := range items {
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
