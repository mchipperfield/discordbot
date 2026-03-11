package main

import "testing"

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

func TestIsKit(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"got a kit", true},
		{"KIT", true},
		{"the kit is ready", true},
		{"kitten", false},   // word boundary — should NOT match
		{"skit", false},     // word boundary — should NOT match
		{"toolkit", false},  // word boundary — should NOT match
		{"my kit.", true},   // punctuation after word
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
		{"fullsend", false},   // no space — should NOT match
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
		{"listening", false},    // word boundary — should NOT match
		{"mislistened", false},  // word boundary — should NOT match
		{"", false},
	}
	for _, c := range cases {
		if got := isListen(c.content); got != c.want {
			t.Errorf("isListen(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}
