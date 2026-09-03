package api

import (
	"testing"
	"time"
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
		if _, err := playWindowTime(value, false); err == nil {
			t.Fatalf("playWindowTime(%q) accepted a value <input type=\"time\"> shows as empty", value)
		}
	}
	if got, err := playWindowTime("09:00", false); err != nil || got != 540 {
		t.Fatalf(`playWindowTime("09:00") = %d, %v; want 540 and no error`, got, err)
	}
	if got, err := playWindowTime("24:00", true); err != nil || got != 1440 {
		t.Fatalf(`playWindowTime("24:00", true) = %d, %v; want 1440 and no error`, got, err)
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

// A window that ends at the minute it begins is empty, and the screen draws it
// as one: two identical times over a switch that is on. It was enforced as the
// whole day, so the policy that looked tightest was no policy at all.
func TestPlayPolicyRefusesAWindowThatEndsWhereItBegins(t *testing.T) {
	for _, raw := range []string{
		`{"enabled":true,"windows":[{"days":[1],"start":"09:00","end":"09:00"}]}`,
		`{"enabled":true,"windows":[{"start":"00:00","end":"00:00"}]}`,
		`{"enabled":true,"windows":[{"days":[1],"start":"09:00","end":"18:00"},{"start":"12:00","end":"12:00"}]}`,
	} {
		if msg := validateSetting("play_policy", []byte(raw)); msg == "" {
			t.Fatalf("validateSetting accepted an empty window: %s", raw)
		}
	}
}

func TestPlayWindowsAllow(t *testing.T) {
	// A Wednesday, so the day before a window that runs past midnight is a
	// Tuesday and the weekday numbers below say which is which.
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 9, 2+day-3, hour, minute, 0, 0, time.UTC)
	}
	weekdays := []int{1, 2, 3, 4, 5}
	for name, tc := range map[string]struct {
		windows []playWindow
		now     time.Time
		want    bool
	}{
		"no window restricts no hour":        {nil, at(3, 4, 0), true},
		"inside the window":                  {[]playWindow{{Days: weekdays, Start: "09:00", End: "18:00"}}, at(3, 12, 0), true},
		"both bounds are inside":             {[]playWindow{{Days: weekdays, Start: "09:00", End: "18:00"}}, at(3, 18, 0), true},
		"before the window":                  {[]playWindow{{Days: weekdays, Start: "09:00", End: "18:00"}}, at(3, 8, 59), false},
		"a day the window does not name":     {[]playWindow{{Days: weekdays, Start: "09:00", End: "18:00"}}, at(6, 12, 0), false},
		"no day named means every day":       {[]playWindow{{Start: "09:00", End: "18:00"}}, at(6, 12, 0), true},
		"any window may allow the hour":      {[]playWindow{{Days: []int{6}, Start: "10:00", End: "12:00"}, {Days: weekdays, Start: "09:00", End: "18:00"}}, at(3, 12, 0), true},
		"a window running past midnight":     {[]playWindow{{Days: []int{2}, Start: "22:00", End: "02:00"}}, at(3, 1, 0), true},
		"and the day it opens on":            {[]playWindow{{Days: []int{2}, Start: "22:00", End: "02:00"}}, at(2, 23, 0), true},
		"but not the morning after that day": {[]playWindow{{Days: []int{2}, Start: "22:00", End: "02:00"}}, at(2, 1, 0), false},
		"to the end of the day":              {[]playWindow{{Start: "18:00", End: "24:00"}}, at(3, 23, 59), true},
		// A bound the clock cannot spell says nothing about when play is
		// allowed, so the window it belongs to allows nothing.
		"an unreadable bound allows nothing": {[]playWindow{{Start: "9:00 PM", End: "18:00"}}, at(3, 12, 0), false},
		// The window equal bounds describe is the minute they name. Reading it
		// as the whole day turned the narrowest policy into no policy at all.
		"an empty window is one minute wide": {[]playWindow{{Days: weekdays, Start: "09:00", End: "09:00"}}, at(3, 9, 0), true},
		"and nothing on either side of it":   {[]playWindow{{Days: weekdays, Start: "09:00", End: "09:00"}}, at(3, 14, 0), false},
	} {
		if got := playWindowsAllow(tc.windows, tc.now); got != tc.want {
			t.Errorf("%s: playWindowsAllow(...) = %v; want %v", name, got, tc.want)
		}
	}
}
