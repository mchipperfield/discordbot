package sd

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

// --- Predicate tests ---------------------------------------------------------

func TestIsQuoteCommand(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"!quote", true},
		{"!Quote", false}, // case-sensitive
		{"!quote extra", false},
		{"quote", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isQuoteCommand(c.content); got != c.want {
			t.Errorf("isQuoteCommand(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestIsHungry(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"I'm hungry", true},
		{"HUNGRY!", true},
		{"so hungry today", true},
		{"I'm full", false},
		{"hungrily ate", false}, // "hungrily" does not contain "hungry"
		{"", false},
	}
	for _, c := range cases {
		if got := isHungry(c.content); got != c.want {
			t.Errorf("isHungry(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestIsTired(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"so tired today", true},
		{"TIRED", true},
		{"feeling tired and sleepy", true},
		{"not sleepy", false},
		{"retired", true}, // substring match is intentional
		{"", false},
	}
	for _, c := range cases {
		if got := isTired(c.content); got != c.want {
			t.Errorf("isTired(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

// --- Register smoke test -----------------------------------------------------

func TestRegister_NoHangs(t *testing.T) {
	s := &discordgo.Session{State: discordgo.NewState()}
	s.State.User = &discordgo.User{ID: "bot-123"}
	Register(s, "guild-123", nil)
}
