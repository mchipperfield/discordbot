package nxg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// GoafAlert holds the configuration for a single bear alert.
type GoafAlert struct {
	Bear          int    `json:"bear"`            // 1 or 2
	Time          string `json:"time"`            // HH:MM UTC
	ReferenceDate string `json:"reference_date"`  // YYYY-MM-DD; a known day this bear fires
	MinutesBefore int    `json:"minutes_before"`
	LastAlertDate string `json:"last_alert_date"` // YYYY-MM-DD; empty = never sent
}

// goafState is the on-disk representation of all configured alerts.
type goafState struct {
	Alerts []*GoafAlert `json:"alerts"`
}

// GoafService manages bear start-time alerts for the NXG server.
// All mutable state is owned here; no package-level globals.
type GoafService struct {
	channelID string
	stateFile string

	// send is called to post a message to Discord. Wired in Register; can be
	// replaced in tests without a live session.
	send func(channelID, msg string)

	// now returns the current time. Defaults to time.Now().UTC(); overridden in tests.
	now func() time.Time

	mu    sync.Mutex
	state goafState
}

// NewGoafService creates a GoafService and loads any existing state from stateFile.
func NewGoafService(channelID, stateFile string) *GoafService {
	g := &GoafService{
		channelID: channelID,
		stateFile: stateFile,
		now:       func() time.Time { return time.Now().UTC() },
	}
	if err := g.loadState(); err != nil {
		slog.Warn("goaf: could not load saved state", "error", err)
	}
	return g
}

// Register wires the /goaf interaction handler onto s and starts the background ticker.
// Must be called once at startup, never inside a Ready callback.
func (g *GoafService) Register(s *discordgo.Session) {
	g.send = func(channelID, msg string) {
		if _, err := s.ChannelMessageSend(channelID, msg); err != nil {
			slog.Error("goaf: failed to send alert", "error", err, "channel", channelID)
		}
	}
	s.AddHandler(g.interactionHandler())
	go g.startTicker()
}

// GoafCommands returns the slash command definitions to be registered in the Ready callback.
func (g *GoafService) GoafCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "bear",
			Description: "Configure a bear alert",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "bear",
					Description: "Which bear",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "1", Value: 1},
						{Name: "2", Value: 2},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "time",
					Description: "Event date and time in UTC — e.g. 2026-03-15 19:00",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "alert_before",
					Description: "How many minutes before the event to send the alert",
					Required:    true,
				},
			},
		},
	}
}

// --- interaction handler -----------------------------------------------------

func (g *GoafService) interactionHandler() func(*discordgo.Session, *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "bear" {
			return
		}

		opts := i.ApplicationCommandData().Options
		bear := int(opts[0].IntValue()) // enforced by Discord to be 1 or 2

		refDate, eventTime, err := parseDateTime(opts[1].StringValue())
		if err != nil {
			goafRespond(s, i, fmt.Sprintf("Error: %v", err))
			return
		}

		minutesBefore := int(opts[2].IntValue())
		if minutesBefore <= 0 {
			goafRespond(s, i, "Error: alert_before must be a positive number of minutes")
			return
		}

		if err := g.configure(bear, refDate, eventTime, minutesBefore); err != nil {
			goafRespond(s, i, fmt.Sprintf("Error: %v", err))
			return
		}

		goafRespond(s, i, fmt.Sprintf(
			"Bear %d configured: %s %s UTC, alerting %d minutes before. The 2-day cycle will be handled automatically.",
			bear, refDate, eventTime, minutesBefore,
		))
	}
}

// configure upserts the alert entry for the given bear.
func (g *GoafService) configure(bear int, refDate, eventTime string, minutesBefore int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, a := range g.state.Alerts {
		if a.Bear == bear {
			a.ReferenceDate = refDate
			a.Time = eventTime
			a.MinutesBefore = minutesBefore
			a.LastAlertDate = "" // reset so the alert fires again on the next occurrence
			return g.saveState()
		}
	}

	g.state.Alerts = append(g.state.Alerts, &GoafAlert{
		Bear:          bear,
		ReferenceDate: refDate,
		Time:          eventTime,
		MinutesBefore: minutesBefore,
	})
	return g.saveState()
}

// --- ticker ------------------------------------------------------------------

func (g *GoafService) startTicker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		g.tick(g.now())
	}
}

// tick inspects every configured alert and fires any that are due.
func (g *GoafService) tick(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	today := now.Format("2006-01-02")

	for _, alert := range g.state.Alerts {
		if alert.LastAlertDate == today {
			continue
		}

		if !isBearDay(today, alert.ReferenceDate) {
			continue
		}

		eventT, err := parseHHMM(alert.Time)
		if err != nil {
			slog.Warn("goaf: bad alert time in state", "bear", alert.Bear, "time", alert.Time)
			continue
		}

		// Anchor event time to today's UTC date.
		eventToday := time.Date(now.Year(), now.Month(), now.Day(), eventT.Hour(), eventT.Minute(), 0, 0, time.UTC)
		alertAt := eventToday.Add(-time.Duration(alert.MinutesBefore) * time.Minute)

		// Fire only inside the window [alertAt, eventToday).
		if now.Before(alertAt) || !now.Before(eventToday) {
			continue
		}

		alert.LastAlertDate = today
		if err := g.saveState(); err != nil {
			slog.Error("goaf: failed to persist state after alert", "error", err)
		}
		g.sendAlert(alert)
	}
}

func (g *GoafService) sendAlert(alert *GoafAlert) {
	msg := fmt.Sprintf("@everyone Bear %d is starting in %d minutes", alert.Bear, alert.MinutesBefore)
	g.send(g.channelID, msg)
}

// --- persistence -------------------------------------------------------------

func (g *GoafService) loadState() error {
	data, err := os.ReadFile(g.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &g.state)
}

// saveState must be called with g.mu held.
func (g *GoafService) saveState() error {
	data, err := json.MarshalIndent(g.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.stateFile, data, 0644)
}

// --- helpers -----------------------------------------------------------------

// parseDateTime parses "YYYY-MM-DD HH:MM" and returns the date and time parts separately.
func parseDateTime(s string) (refDate, eventTime string, err error) {
	t, err := time.Parse("2006-01-02 15:04", strings.TrimSpace(s))
	if err != nil {
		return "", "", fmt.Errorf("invalid time %q — expected YYYY-MM-DD HH:MM (e.g. 2026-03-15 19:00)", s)
	}
	return t.Format("2006-01-02"), t.Format("15:04"), nil
}

// parseHHMM parses a "HH:MM" string.
func parseHHMM(s string) (time.Time, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q — expected HH:MM", s)
	}
	return t, nil
}

// isBearDay reports whether today is a bear day given a reference date.
// A bear fires every 2 days from its reference date.
func isBearDay(today, referenceDate string) bool {
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return false
	}
	ref, err := time.Parse("2006-01-02", referenceDate)
	if err != nil {
		return false
	}
	days := int(t.Sub(ref).Hours() / 24)
	return days >= 0 && days%2 == 0
}

// goafRespond sends an immediate reply to a Discord interaction.
func goafRespond(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	}); err != nil {
		slog.Error("goaf: failed to respond to interaction", "error", err)
	}
}
