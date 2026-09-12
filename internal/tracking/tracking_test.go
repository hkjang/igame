package tracking

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func boolPtr(value bool) *bool { return &value }

func TestZeroConfigTracksNothing(t *testing.T) {
	// A deployment that never saved the setting must behave as before the
	// setting existed: no snippet on any page, nothing added to the policy.
	var config Config
	if err := json.Unmarshal([]byte(`{}`), &config); err != nil {
		t.Fatal(err)
	}
	config = config.Normalized()
	for _, path := range []string{"/", "/games/snake", "/admin/settings"} {
		if config.Active(path) {
			t.Fatalf("zero config is active on %s", path)
		}
	}
	scripts, connects, images := config.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("zero config added policy sources: %v %v %v", scripts, connects, images)
	}
	if config.ProxyTarget() != "" {
		t.Fatalf("zero config forwards the proxy to %q", config.ProxyTarget())
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("zero config does not validate: %v", err)
	}
}

func TestMomentoDefaultsToTheSameOriginProxy(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.corp.example/", MomentoSiteID: "igame"}.Normalized()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	snippet := config.Snippet("n0nce")
	for _, want := range []string{`src="/momento/tracker.js"`, `data-endpoint="/momento"`, `data-site-id="igame"`, `data-environment="prd"`, `data-contract-version="1"`, `nonce="n0nce"`} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("snippet lacks %s: %s", want, snippet)
		}
	}
	if strings.Contains(snippet, "momento.corp.example") {
		t.Fatalf("proxied snippet names the collector: %s", snippet)
	}
	scripts, connects, images := config.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("proxied Momento needs no external origin, got %v %v %v", scripts, connects, images)
	}
	if got := config.ProxyTarget(); got != "https://momento.corp.example" {
		t.Fatalf("proxy target = %q", got)
	}
}

func TestMomentoDirectNamesTheCollectorInThePolicy(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.corp.example", MomentoSiteID: "igame", MomentoEnvironment: "stg", MomentoProxy: boolPtr(false)}.Normalized()
	snippet := config.Snippet("n0nce")
	if !strings.Contains(snippet, `src="https://momento.corp.example/tracker.js"`) || strings.Contains(snippet, "data-endpoint") {
		t.Fatalf("direct snippet = %s", snippet)
	}
	if !strings.Contains(snippet, `data-environment="stg"`) {
		t.Fatalf("environment not carried: %s", snippet)
	}
	scripts, connects, _ := config.PolicySources()
	if len(scripts) != 1 || scripts[0] != "https://momento.corp.example" || len(connects) != 1 {
		t.Fatalf("policy sources = %v %v", scripts, connects)
	}
	if config.ProxyTarget() != "" {
		t.Fatal("direct Momento still forwards the proxy")
	}
}

func TestAdminScreensNeedIncludeAdmin(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://m.example", MomentoSiteID: "1"}.Normalized()
	if !config.Active("/") || !config.Active("/games/snake") || !config.Active("/administration") {
		t.Fatal("portal pages are not tracked")
	}
	if config.Active("/admin") || config.Active("/admin/settings") {
		t.Fatal("admin screens tracked without include_admin")
	}
	config.IncludeAdmin = true
	if !config.Active("/admin/settings") {
		t.Fatal("include_admin did not open the admin screens")
	}
}

func TestDisabledOrNoneIsInactiveWhateverElseIsSet(t *testing.T) {
	config := Config{Enabled: false, Provider: ProviderGA4, MeasurementID: "G-1"}.Normalized()
	if config.Active("/") {
		t.Fatal("disabled config is active")
	}
	config = Config{Enabled: true, Provider: ProviderNone, CustomSnippet: "<script></script>"}.Normalized()
	if config.Active("/") {
		t.Fatal("provider none is active")
	}
}

func TestEveryScriptTagGetsTheNonce(t *testing.T) {
	snippet := `<SCRIPT async src="https://t.example/a.js"></SCRIPT>
<script nonce="theirs">x()</script>
<script>y()</script>`
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: snippet}.Normalized()
	out := config.Snippet("abc")
	if strings.Count(out, `nonce="abc"`) != 2 {
		t.Fatalf("nonce applied %d times: %s", strings.Count(out, `nonce="abc"`), out)
	}
	if !strings.Contains(out, `nonce="theirs"`) {
		t.Fatalf("existing nonce was replaced: %s", out)
	}
	if !strings.Contains(out, `<SCRIPT nonce="abc" async`) {
		t.Fatalf("upper-case tag skipped: %s", out)
	}
	for _, provider := range []string{ProviderGA4, ProviderGTM, ProviderMatomo} {
		config := Config{Enabled: true, Provider: provider, MeasurementID: "G-1", MatomoURL: "https://matomo.example", MatomoSiteID: "3"}.Normalized()
		out := config.Snippet("abc")
		if strings.Count(out, "<script") != strings.Count(out, `nonce="abc"`) {
			t.Fatalf("%s: %d script tags, %d nonces: %s", provider, strings.Count(out, "<script"), strings.Count(out, `nonce="abc"`), out)
		}
	}
}

func TestConfiguredValuesCannotBreakOutOfTheSnippet(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderGA4, MeasurementID: `G-1');alert(1);//</script><script>`}.Normalized()
	out := config.Snippet("abc")
	if strings.Count(out, "<script") != 2 || strings.Contains(out, "</script><script>") {
		t.Fatalf("measurement id escaped the literal: %s", out)
	}
	config = Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://m.example", MomentoSiteID: `x" onload="alert(1)`}.Normalized()
	if out := config.Snippet("abc"); strings.Contains(out, `onload="`) {
		t.Fatalf("site id escaped the attribute: %s", out)
	}
}

func TestSnippetOriginsAreReadFromThePastedCode(t *testing.T) {
	snippet := `<script src="https://tracker.corp.example/t.js"></script>
<script>window.__t={endpoint:"https://tracker.corp.example/collect/v1/events",pixel:'http://pixel.corp.example:8080/p.gif?id=1'};
fetch(window.__t.endpoint+'/x');</script>`
	origins := SnippetOrigins(snippet)
	if len(origins) != 2 || origins[0] != "https://tracker.corp.example" || origins[1] != "http://pixel.corp.example:8080" {
		t.Fatalf("origins = %v", origins)
	}
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: snippet, AllowedHosts: "https://extra.example, https://tracker.corp.example\nhttps://*.wild.example"}.Normalized()
	scripts, connects, images := config.PolicySources()
	want := []string{"https://tracker.corp.example", "http://pixel.corp.example:8080", "https://extra.example", "https://*.wild.example"}
	for _, group := range [][]string{scripts, connects, images} {
		if strings.Join(group, " ") != strings.Join(want, " ") {
			t.Fatalf("policy sources = %v, want %v", group, want)
		}
	}
}

func TestOriginRefusesWhatCannotGoInAHeader(t *testing.T) {
	for _, raw := range []string{"", "inline", "data:text/html,x", "chrome-extension://abc/x.js", "ftp://x.example", "https://", "//host.example/x", "https://a.example; script-src *"} {
		if origin, ok := Origin(raw); ok {
			t.Fatalf("Origin(%q) = %q, want rejection", raw, origin)
		}
	}
	if origin, ok := Origin("HTTPS://Momento.Corp.Example:8443/collect?x=1"); !ok || origin != "https://momento.corp.example:8443" {
		t.Fatalf("Origin = %q %v", origin, ok)
	}
}

func TestValidateNamesWhatIsMissing(t *testing.T) {
	tests := map[string]Config{
		"momento without site id":    {Enabled: true, Provider: ProviderMomento, MomentoURL: "https://m.example"},
		"momento with a bare host":   {Enabled: true, Provider: ProviderMomento, MomentoURL: "m.example", MomentoSiteID: "1"},
		"ga4 without id":             {Enabled: true, Provider: ProviderGA4},
		"matomo without url":         {Enabled: true, Provider: ProviderMatomo, MatomoSiteID: "1"},
		"custom without snippet":     {Enabled: true, Provider: ProviderCustom},
		"unknown provider":           {Enabled: true, Provider: "pixel"},
		"snippet over 8KB":           {Provider: ProviderCustom, CustomSnippet: "<script>" + strings.Repeat("x", MaxSnippetBytes) + "</script>"},
		"allowed host not an origin": {Provider: ProviderNone, AllowedHosts: "tracker.example"},
	}
	for name, config := range tests {
		if err := config.Normalized().Validate(); err == nil {
			t.Errorf("%s: validated", name)
		}
	}
	// A provider that is filled in but switched off still has to validate, so a
	// half-typed setting can be saved and finished later.
	if err := (Config{Enabled: false, Provider: ProviderMomento, MomentoURL: "https://m.example", MomentoSiteID: "1"}).Normalized().Validate(); err != nil {
		t.Fatalf("complete but disabled config refused: %v", err)
	}
	if err := (Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: `<script src="https://t.example/t.js"></script>`}).Normalized().Validate(); err != nil {
		t.Fatalf("custom config refused: %v", err)
	}
}

func TestAddAllowedHostKeepsTheListTidy(t *testing.T) {
	if got := AddAllowedHost("", "https://a.example/path"); got != "https://a.example" {
		t.Fatalf("first entry = %q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://A.example/"); got != "https://a.example" {
		t.Fatalf("duplicate added: %q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://b.example"); got != "https://a.example, https://b.example" {
		t.Fatalf("second entry = %q", got)
	}
	if got := AddAllowedHost("https://a.example", "inline"); got != "https://a.example" {
		t.Fatalf("non-origin added: %q", got)
	}
}

func TestRecorderKeepsDistinctOriginsNotCounts(t *testing.T) {
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	recorder := NewRecorder(func() time.Time { return now })
	for i := 0; i < 5; i++ {
		recorder.Record("https://momento.corp.example/collect/v1/events", "connect-src", "https://igame.corp.example/")
	}
	recorder.Record("inline", "script-src-elem", "/")
	recorder.Record("chrome-extension://abc/x.js", "script-src", "/")
	items := recorder.List(Config{})
	if len(items) != 1 || items[0].Origin != "https://momento.corp.example" || items[0].Count != 5 || items[0].Directive != "connect-src" || items[0].Allowed {
		t.Fatalf("items = %+v", items)
	}
	// The same origin under a second directive is a second thing to allow.
	now = now.Add(time.Minute)
	recorder.Record("https://momento.corp.example/tracker.js", "script-src-elem 'self'", "/")
	items = recorder.List(Config{})
	if len(items) != 2 || items[0].Directive != "script-src-elem" || items[1].Directive != "connect-src" {
		t.Fatalf("items = %+v", items)
	}
	allowed := recorder.List(Config{Enabled: true, Provider: ProviderNone, AllowedHosts: "https://momento.corp.example"}.Normalized())
	if !allowed[0].Allowed || !allowed[1].Allowed {
		t.Fatalf("allow list not reflected: %+v", allowed)
	}
	wild := recorder.List(Config{AllowedHosts: "https://*.corp.example"}.Normalized())
	if !wild[0].Allowed {
		t.Fatalf("wildcard not reflected: %+v", wild)
	}
	recorder.Forget()
	if len(recorder.List(Config{})) != 0 {
		t.Fatal("Forget left reports behind")
	}
}

func TestRecorderIsBounded(t *testing.T) {
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	recorder := NewRecorder(func() time.Time { now = now.Add(time.Second); return now })
	for i := 0; i < MaxViolations+10; i++ {
		recorder.Record("https://host"+string(rune('a'+i%26))+strings.Repeat("x", i/26)+".example/x", "connect-src", "/")
	}
	items := recorder.List(Config{})
	if len(items) != MaxViolations {
		t.Fatalf("recorder holds %d, want %d", len(items), MaxViolations)
	}
	if items[len(items)-1].Origin == "https://hosta.example" {
		t.Fatal("the oldest report survived eviction")
	}
}
