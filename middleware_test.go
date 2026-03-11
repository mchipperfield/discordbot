package main

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

const (
	testGuildID = "guild-123"
	testBotID   = "bot-456"
	testUserID  = "user-789"
)

func newTestSession() *discordgo.Session {
	s := &discordgo.Session{State: discordgo.NewState()}
	s.State.User = &discordgo.User{ID: testBotID}
	return s
}

func newTestMessage(guildID, authorID, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			GuildID: guildID,
			Content: content,
			Author:  &discordgo.User{ID: authorID},
		},
	}
}

func TestOnMessage_WrongGuild(t *testing.T) {
	fired := false
	h := onMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})

	h(newTestSession(), newTestMessage("other-guild", testUserID, "hello"))

	if fired {
		t.Error("handler should not fire for a different guild")
	}
}

func TestOnMessage_BotSelf(t *testing.T) {
	fired := false
	h := onMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})

	s := newTestSession()
	h(s, newTestMessage(testGuildID, testBotID, "hello"))

	if fired {
		t.Error("handler should not fire for the bot's own messages")
	}
}

func TestOnMessage_Fires(t *testing.T) {
	fired := false
	h := onMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})

	s := newTestSession()
	h(s, newTestMessage(testGuildID, testUserID, "hello"))

	if !fired {
		t.Error("handler should fire for a real user message in the correct guild")
	}
}

func TestOnAnyMessage_BotSelf(t *testing.T) {
	fired := false
	h := onAnyMessage(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})

	s := newTestSession()
	h(s, newTestMessage(testGuildID, testBotID, "hello"))

	if fired {
		t.Error("handler should not fire for the bot's own messages")
	}
}

func TestOnAnyMessage_Fires(t *testing.T) {
	fired := false
	h := onAnyMessage(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})

	s := newTestSession()
	h(s, newTestMessage("any-guild", testUserID, "hello"))

	if !fired {
		t.Error("handler should fire for a real user message regardless of guild")
	}
}

func TestOnMessage_PassesSessionAndMessage(t *testing.T) {
	var gotGuildID, gotContent string
	h := onMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		gotGuildID = m.GuildID
		gotContent = m.Content
	})

	s := newTestSession()
	h(s, newTestMessage(testGuildID, testUserID, "test content"))

	if gotGuildID != testGuildID {
		t.Errorf("got guildID %q, want %q", gotGuildID, testGuildID)
	}
	if gotContent != "test content" {
		t.Errorf("got content %q, want %q", gotContent, "test content")
	}
}
