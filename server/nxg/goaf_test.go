package nxg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// newTestGoafService returns a GoafService backed by a temp-dir state file.
func newTestGoafService(t *testing.T) *GoafService {
	t.Helper()
	stateFile := filepath.Join(t.TempDir(), "goaf_state.json")
	return NewGoafService("chan-test", stateFile)
}

func stubSend(t *testing.T, msgs *[]string) func(string, string) {
	t.Helper()
	return func(_, msg string) {
		*msgs = append(*msgs, msg)
	}
}

// mustConfigure is a test helper that calls configure and fails on error.
func mustConfigure(t *testing.T, g *GoafService, bear int, refDate, eventTime string, minutesBefore int) {
	t.Helper()
	if err := g.configure(bear, refDate, eventTime, minutesBefore); err != nil {
		t.Fatalf("configure(%d, %q, %q, %d) unexpected error: %v", bear, refDate, eventTime, minutesBefore, err)
	}
}

// --- parseDateTime -----------------------------------------------------------

func TestParseDateTime_Valid(t *testing.T) {
	cases := []struct {
		input         string
		wantDate      string
		wantTime      string
	}{
		{"2026-03-15 19:00", "2026-03-15", "19:00"},
		{"2026-03-16 00:10", "2026-03-16", "00:10"},
		{"2026-12-31 23:59", "2026-12-31", "23:59"},
		{" 2026-03-15 19:00 ", "2026-03-15", "19:00"}, // leading/trailing space
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			gotDate, gotTime, err := parseDateTime(c.input)
			if err != nil {
				t.Fatalf("parseDateTime(%q) unexpected error: %v", c.input, err)
			}
			if gotDate != c.wantDate {
				t.Errorf("date = %q, want %q", gotDate, c.wantDate)
			}
			if gotTime != c.wantTime {
				t.Errorf("time = %q, want %q", gotTime, c.wantTime)
			}
		})
	}
}

func TestParseDateTime_Invalid(t *testing.T) {
	cases := []string{
		"",
		"2026-03-15",          // missing time
		"19:00",               // missing date
		"15-03-2026 19:00",    // wrong date format
		"2026-03-15T19:00",    // wrong separator
		"2026-03-15 25:00",    // invalid hour
		"2026-03-15 19:60",    // invalid minute
		"notadate",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			if _, _, err := parseDateTime(input); err == nil {
				t.Errorf("parseDateTime(%q) expected error, got nil", input)
			}
		})
	}
}

// --- configure ---------------------------------------------------------------

func TestGoafService_Configure_StoresAlert(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)

	a := g.alertForLocked(1)
	if a == nil {
		t.Fatal("expected alert stored for bear 1")
	}
	if a.ReferenceDate != "2026-03-15" {
		t.Errorf("ReferenceDate = %q, want 2026-03-15", a.ReferenceDate)
	}
	if a.Time != "19:00" {
		t.Errorf("Time = %q, want 19:00", a.Time)
	}
	if a.MinutesBefore != 10 {
		t.Errorf("MinutesBefore = %d, want 10", a.MinutesBefore)
	}
}

func TestGoafService_Configure_UpdatesExistingAlert(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 1, "2026-03-17", "20:00", 5)

	if len(g.state.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(g.state.Alerts))
	}
	a := g.state.Alerts[0]
	if a.ReferenceDate != "2026-03-17" || a.Time != "20:00" || a.MinutesBefore != 5 {
		t.Errorf("unexpected updated alert: %+v", a)
	}
}

func TestGoafService_Configure_ResetsLastAlertDate(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	g.state.Alerts[0].LastAlertDate = "2026-03-15"

	mustConfigure(t, g, 1, "2026-03-17", "20:00", 5)

	if g.state.Alerts[0].LastAlertDate != "" {
		t.Errorf("expected LastAlertDate reset, got %q", g.state.Alerts[0].LastAlertDate)
	}
}

func TestGoafService_Configure_TwoBears(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 2, "2026-03-16", "00:10", 10)

	if len(g.state.Alerts) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(g.state.Alerts))
	}
}

// --- isBearDay ---------------------------------------------------------------

func TestIsBearDay(t *testing.T) {
	ref := "2026-03-15"

	cases := []struct {
		today string
		want  bool
	}{
		{"2026-03-15", true},  // day 0 — reference day
		{"2026-03-16", false}, // day 1 — off cycle
		{"2026-03-17", true},  // day 2 — on cycle
		{"2026-03-18", false}, // day 3 — off cycle
		{"2026-03-19", true},  // day 4 — on cycle
		{"2026-03-14", false}, // before reference
	}

	for _, c := range cases {
		t.Run(c.today, func(t *testing.T) {
			if got := isBearDay(c.today, ref); got != c.want {
				t.Errorf("isBearDay(%q, %q) = %v, want %v", c.today, ref, got, c.want)
			}
		})
	}
}

func TestIsBearDay_IndependentPerBear(t *testing.T) {
	bear1Ref := "2026-03-15"
	bear2Ref := "2026-03-16"

	days := []struct {
		date      string
		wantBear1 bool
		wantBear2 bool
	}{
		{"2026-03-15", true, false},
		{"2026-03-16", false, true},
		{"2026-03-17", true, false},
		{"2026-03-18", false, true},
	}

	for _, d := range days {
		if got := isBearDay(d.date, bear1Ref); got != d.wantBear1 {
			t.Errorf("bear1 isBearDay(%q) = %v, want %v", d.date, got, d.wantBear1)
		}
		if got := isBearDay(d.date, bear2Ref); got != d.wantBear2 {
			t.Errorf("bear2 isBearDay(%q) = %v, want %v", d.date, got, d.wantBear2)
		}
	}
}

func TestIsBearDay_SameDayBothBears(t *testing.T) {
	// Game pushes both bears to same day — same reference date is valid
	ref := "2026-03-15"
	if !isBearDay("2026-03-15", ref) {
		t.Error("expected bear day on reference date")
	}
}

// --- tick --------------------------------------------------------------------

func TestGoafService_Tick_SendsAlertOnBearDay(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10) // window [09:50, 10:00)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC))

	if len(msgs) != 1 {
		t.Fatalf("expected 1 alert on bear day, got %d", len(msgs))
	}
}

func TestGoafService_Tick_SkipsNonBearDay(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 16, 9, 52, 0, 0, time.UTC)) // day 1 — not a bear day

	if len(msgs) != 0 {
		t.Errorf("expected no alert on non-bear day, got %d", len(msgs))
	}
}

func TestGoafService_Tick_FiresEveryTwoDays(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)

	g.tick(time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC)) // day 0 — fires
	g.tick(time.Date(2026, 3, 16, 9, 52, 0, 0, time.UTC)) // day 1 — skips
	g.tick(time.Date(2026, 3, 17, 9, 52, 0, 0, time.UTC)) // day 2 — fires
	g.tick(time.Date(2026, 3, 18, 9, 52, 0, 0, time.UTC)) // day 3 — skips
	g.tick(time.Date(2026, 3, 19, 9, 52, 0, 0, time.UTC)) // day 4 — fires

	if len(msgs) != 3 {
		t.Errorf("expected 3 alerts over 5 days, got %d", len(msgs))
	}
}

func TestGoafService_Tick_TwoBearsAlternateDays(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 2, "2026-03-16", "00:10", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)

	g.tick(time.Date(2026, 3, 15, 18, 52, 0, 0, time.UTC)) // bear 1 day
	g.tick(time.Date(2026, 3, 16, 0, 2, 0, 0, time.UTC))   // bear 2 day
	g.tick(time.Date(2026, 3, 17, 18, 52, 0, 0, time.UTC)) // bear 1 day
	g.tick(time.Date(2026, 3, 18, 0, 2, 0, 0, time.UTC))   // bear 2 day

	if len(msgs) != 4 {
		t.Errorf("expected 4 alerts (2 per bear), got %d", len(msgs))
	}
}

func TestGoafService_Tick_TwoBearsOnSameDay(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 2, "2026-03-15", "22:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)

	g.tick(time.Date(2026, 3, 15, 18, 52, 0, 0, time.UTC)) // bear 1 window
	g.tick(time.Date(2026, 3, 15, 21, 52, 0, 0, time.UTC)) // bear 2 window

	if len(msgs) != 2 {
		t.Errorf("expected 2 alerts when both bears on same day, got %d", len(msgs))
	}
}

func TestGoafService_Tick_DoesNotSendBeforeWindow(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 15, 9, 49, 0, 0, time.UTC))

	if len(msgs) != 0 {
		t.Errorf("expected no alert before window, got %d", len(msgs))
	}
}

func TestGoafService_Tick_DoesNotSendAfterEvent(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))
	g.tick(time.Date(2026, 3, 15, 10, 5, 0, 0, time.UTC))

	if len(msgs) != 0 {
		t.Errorf("expected no alert after event, got %d", len(msgs))
	}
}

func TestGoafService_Tick_SendsOnlyOncePerDay(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)

	base := time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC)
	g.tick(base)
	g.tick(base.Add(time.Minute))
	g.tick(base.Add(2 * time.Minute))

	if len(msgs) != 1 {
		t.Errorf("expected exactly 1 alert per day, got %d", len(msgs))
	}
}

func TestGoafService_Tick_MessageContent_Bear1(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC))

	want := "@everyone Bear 1 is starting in 10 minutes"
	if len(msgs) == 0 || msgs[0] != want {
		t.Errorf("message = %q, want %q", msgs, want)
	}
}

func TestGoafService_Tick_MessageContent_Bear2(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 2, "2026-03-16", "22:00", 15)

	var msgs []string
	g.send = stubSend(t, &msgs)
	g.tick(time.Date(2026, 3, 16, 21, 50, 0, 0, time.UTC))

	want := "@everyone Bear 2 is starting in 15 minutes"
	if len(msgs) == 0 || msgs[0] != want {
		t.Errorf("message = %q, want %q", msgs, want)
	}
}

// --- persistence -------------------------------------------------------------

func TestGoafService_Persistence_LoadsStateFromDisk(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "goaf_state.json")

	g1 := NewGoafService("chan-test", stateFile)
	mustConfigure(t, g1, 1, "2026-03-15", "19:00", 10)

	g2 := NewGoafService("chan-test", stateFile)
	a := g2.alertForLocked(1)
	if a == nil {
		t.Fatal("expected alert to survive restart")
	}
	if a.ReferenceDate != "2026-03-15" || a.Time != "19:00" || a.MinutesBefore != 10 {
		t.Errorf("loaded alert mismatch: %+v", a)
	}
}

func TestGoafService_Persistence_DoesNotFireAfterRestart(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "goaf_state.json")

	g1 := NewGoafService("chan-test", stateFile)
	mustConfigure(t, g1, 1, "2026-03-15", "10:00", 10)

	count := 0
	g1.send = func(_, _ string) { count++ }
	g1.tick(time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC))

	g2 := NewGoafService("chan-test", stateFile)
	g2.send = func(_, _ string) { count++ }
	g2.tick(time.Date(2026, 3, 15, 9, 53, 0, 0, time.UTC))

	if count != 1 {
		t.Errorf("expected exactly 1 alert across restart, got %d", count)
	}
}

func TestGoafService_Persistence_FreshStartNoFile(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "does_not_exist.json")

	g := NewGoafService("chan-test", stateFile)
	if len(g.state.Alerts) != 0 {
		t.Errorf("expected empty state on first run, got %d alerts", len(g.state.Alerts))
	}
}

func TestGoafService_Persistence_CorruptedFileIsHandledGracefully(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "goaf_state.json")
	if err := os.WriteFile(stateFile, []byte("not valid json {{{{"), 0644); err != nil {
		t.Fatalf("could not write corrupt file: %v", err)
	}

	g := NewGoafService("chan-test", stateFile)
	if len(g.state.Alerts) != 0 {
		t.Errorf("expected empty state after corrupt file, got %d alerts", len(g.state.Alerts))
	}
}

func TestGoafService_Persistence_LastAlertDateWrittenToDiskByTick(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "goaf_state.json")

	g := NewGoafService("chan-test", stateFile)
	mustConfigure(t, g, 1, "2026-03-15", "10:00", 10)
	g.send = func(_, _ string) {}
	g.tick(time.Date(2026, 3, 15, 9, 52, 0, 0, time.UTC))

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("could not read state file: %v", err)
	}
	var state goafState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("invalid JSON after tick: %v", err)
	}
	if len(state.Alerts) == 0 || state.Alerts[0].LastAlertDate != "2026-03-15" {
		t.Errorf("LastAlertDate not written to disk; got %q", state.Alerts[0].LastAlertDate)
	}
}

func TestGoafService_Persistence_StateFileIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "goaf_state.json")

	g := NewGoafService("chan-test", stateFile)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 2, "2026-03-16", "00:10", 5)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("could not read state file: %v", err)
	}
	var state goafState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("state file contains invalid JSON: %v", err)
	}
	if len(state.Alerts) != 2 {
		t.Errorf("expected 2 alerts in JSON, got %d", len(state.Alerts))
	}
}

// --- formatDuration ----------------------------------------------------------

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "< 1min"},
		{30 * time.Second, "< 1min"},
		{1 * time.Minute, "1min"},
		{45 * time.Minute, "45min"},
		{90 * time.Minute, "1h 30min"},
		{3*time.Hour + 45*time.Minute, "3h 45min"},
		{24 * time.Hour, "1d"},
		{25 * time.Hour, "1d 1h"},
		{2*24*time.Hour + 3*time.Hour + 45*time.Minute, "2d 3h 45min"},
	}

	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			if got := formatDuration(c.d); got != c.want {
				t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
			}
		})
	}
}

// --- status ------------------------------------------------------------------

func TestGoafService_Status_NotConfigured(t *testing.T) {
	g := newTestGoafService(t)

	status := g.buildStatus(time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))

	if !contains(status, "Bear 1 — not configured") {
		t.Errorf("expected Bear 1 not configured, got:\n%s", status)
	}
	if !contains(status, "Bear 2 — not configured") {
		t.Errorf("expected Bear 2 not configured, got:\n%s", status)
	}
}

func TestGoafService_Status_AlertInFuture(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10) // alert at 18:50

	// 10:00 UTC — 8h 50min before the alert
	status := g.buildStatus(time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))

	if !contains(status, "Bear 1 — alert in 8h 50min (today, event 19:00 UTC)") {
		t.Errorf("unexpected status:\n%s", status)
	}
}

func TestGoafService_Status_AlertInFutureNextBearDay(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)

	// 16 March — not a bear1 day; next is 17 March
	now := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC)
	status := g.buildStatus(now)

	if !contains(status, "Bear 1 — alert in 1d 8h 50min (2026-03-17, event 19:00 UTC)") {
		t.Errorf("unexpected status:\n%s", status)
	}
}

func TestGoafService_Status_InAlertWindow(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10) // alert window [18:50, 19:00)

	// 18:55 — inside the window
	status := g.buildStatus(time.Date(2026, 3, 15, 18, 55, 0, 0, time.UTC))

	if !contains(status, "Bear 1 — alert firing now (today, event 19:00 UTC)") {
		t.Errorf("unexpected status:\n%s", status)
	}
}

func TestGoafService_Status_AlreadySentToday(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	g.state.Alerts[0].LastAlertDate = "2026-03-15"

	// 18:55 — in window but already sent; should show next occurrence (17 March)
	status := g.buildStatus(time.Date(2026, 3, 15, 18, 55, 0, 0, time.UTC))

	if !contains(status, "2026-03-17") {
		t.Errorf("expected next occurrence on 2026-03-17, got:\n%s", status)
	}
}

func TestGoafService_Status_AfterEventToday(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)

	// 20:00 — event has passed; should show next occurrence (17 March)
	status := g.buildStatus(time.Date(2026, 3, 15, 20, 0, 0, 0, time.UTC))

	if !contains(status, "2026-03-17") {
		t.Errorf("expected next occurrence on 2026-03-17, got:\n%s", status)
	}
}

func TestGoafService_Status_BothBears(t *testing.T) {
	g := newTestGoafService(t)
	mustConfigure(t, g, 1, "2026-03-15", "19:00", 10)
	mustConfigure(t, g, 2, "2026-03-16", "00:10", 10)

	// 15 March 10:00 — bear1 alert is today, bear2 alert is tomorrow
	status := g.buildStatus(time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC))

	if !contains(status, "Bear 1") || !contains(status, "Bear 2") {
		t.Errorf("expected both bears in status:\n%s", status)
	}
	if !contains(status, "today") {
		t.Errorf("expected 'today' for Bear 1:\n%s", status)
	}
	if !contains(status, "2026-03-16") {
		t.Errorf("expected 2026-03-16 for Bear 2:\n%s", status)
	}
}

// contains is a simple substring helper for status assertions.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// --- Register smoke test -----------------------------------------------------

func TestGoafService_Register_NoHangs(t *testing.T) {
	s := &discordgo.Session{State: discordgo.NewState()}
	s.State.User = &discordgo.User{ID: "bot-123"}

	g := newTestGoafService(t)
	g.Register(s)
}
