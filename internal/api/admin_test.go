package api

import (
	"testing"
)

// The play window decides when a colleague may open a game at all, and its two
// bounds were read with strconv.Atoi, which accepts a sign and any amount of
// padding. "+9:+5" and "0009:0005" both became 09:05: a window nobody typed,
// enforced against everyone, and shown back as an empty field because the
// <input type="time"> that sets it can display none of those spellings.
func TestAPlayWindowBoundIsOnlyAClockTime(t *testing.T) {
	for value, want := range map[string]int{
		"00:00": 0,
		"09:05": 545,
		"23:59": 1439,
	} {
		got, err := clockMinutes(value, false)
		if err != nil || got != want {
			t.Fatalf("clockMinutes(%q) = %d, %v; want %d and no error", value, got, err, want)
		}
	}
	for _, value := range []string{
		"+9:+5",
		"-1:00",
		"0009:0005",
		"09:5x",
		" 9:05",
		"09:05 ",
		"09:05:00",
		"0905",
		"09:",
		":05",
		"",
		"24:00",
		"09:60",
	} {
		if got, err := clockMinutes(value, false); err == nil {
			t.Fatalf("clockMinutes(%q) returned %d; want a refusal", value, got)
		}
	}
	// Midnight at the far end closes a window that runs to the end of the day,
	// and only that end of a window may say it.
	if got, err := clockMinutes("24:00", true); err != nil || got != 1440 {
		t.Fatalf(`clockMinutes("24:00", true) = %d, %v; want 1440 and no error`, got, err)
	}
	if got, err := clockMinutes("24:01", true); err == nil {
		t.Fatalf(`clockMinutes("24:01", true) returned %d; want a refusal`, got)
	}
}

// What is stored has to be what the field can show. A bound saved as "9:00" is
// a time the server enforces and the admin screen renders as blank, so the next
// administrator sees no window over a policy that is still turned on.
func TestAStoredPlayWindowIsShownBackByTheFieldThatSetIt(t *testing.T) {
	for _, value := range []string{"9:00", "9:5", "09:5"} {
		if err := playWindowTime(value, false); err == nil {
			t.Fatalf("playWindowTime(%q) accepted a value <input type=\"time\"> shows as empty", value)
		}
	}
	if err := playWindowTime("09:00", false); err != nil {
		t.Fatalf(`playWindowTime("09:00") = %v; want it accepted`, err)
	}
	if err := playWindowTime("24:00", true); err != nil {
		t.Fatalf(`playWindowTime("24:00", true) = %v; want it accepted`, err)
	}
}

func TestPlayPolicyRefusesAWindowItCannotShowBack(t *testing.T) {
	const canonical = `{"enabled":true,"windows":[{"days":[1],"start":"09:00","end":"18:00"}],"daily_limits":{"snake":30}}`
	if msg := validateSetting("play_policy", []byte(canonical)); msg != "" {
		t.Fatalf("validateSetting refused a well-formed policy: %s", msg)
	}
	for _, raw := range []string{
		`{"enabled":true,"windows":[{"start":"+9:00","end":"18:00"}]}`,
		`{"enabled":true,"windows":[{"start":"9:00","end":"18:00"}]}`,
		`{"enabled":true,"windows":[{"start":"09:00","end":"0018:00"}]}`,
	} {
		if msg := validateSetting("play_policy", []byte(raw)); msg == "" {
			t.Fatalf("validateSetting accepted %s", raw)
		}
	}
}
