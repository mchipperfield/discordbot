package nxg

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

// --- Predicate tests ---------------------------------------------------------

func TestIsKit(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"got a kit", true},
		{"KIT", true},
		{"the kit is ready", true},
		{"kitten", false},  // word boundary — should NOT match
		{"skit", false},    // word boundary — should NOT match
		{"toolkit", false}, // word boundary — should NOT match
		{"my kit.", true},  // punctuation after word
		{"", false},
	}
	for _, c := range cases {
		if got := isKit(c.content); got != c.want {
			t.Errorf("isKit(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestIsFullSend(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"full send it", true},
		{"FULL SEND", true},
		{"let's full send this", true},
		{"fullsend", false},    // no space — should NOT match
		{"half send", false},
		{"full sender", false}, // word boundary after
		{"", false},
	}
	for _, c := range cases {
		if got := isFullSend(c.content); got != c.want {
			t.Errorf("isFullSend(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestIsListen(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"listen up", true},
		{"LISTEN", true},
		{"hey listen!", true},
		{"listening", false},   // word boundary — should NOT match
		{"mislistened", false}, // word boundary — should NOT match
		{"", false},
	}
	for _, c := range cases {
		if got := isListen(c.content); got != c.want {
			t.Errorf("isListen(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestMentionsKnownUser(t *testing.T) {
	make := func(ids ...string) *discordgo.MessageCreate {
		msg := &discordgo.MessageCreate{Message: &discordgo.Message{}}
		for _, id := range ids {
			msg.Mentions = append(msg.Mentions, &discordgo.User{ID: id})
		}
		return msg
	}

	cases := []struct {
		msg  *discordgo.MessageCreate
		want bool
	}{
		{make("845223962052788224"), true},           // kinslayer mentioned
		{make("845223962052788224", "999"), true},    // kinslayer + other
		{make("999"), false},                         // unknown user
		{make(), false},                              // no mentions
	}
	for _, c := range cases {
		if got := mentionsKnownUser(c.msg); got != c.want {
			t.Errorf("mentionsKnownUser(%v mentions) = %v, want %v", len(c.msg.Mentions), got, c.want)
		}
	}
}

// --- Register smoke test -----------------------------------------------------

func TestRegister_NoHangs(t *testing.T) {
	s := &discordgo.Session{State: discordgo.NewState()}
	s.State.User = &discordgo.User{ID: "bot-123"}
	Register(s, "guild-123", nil, nil)
}
