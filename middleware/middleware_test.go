package middleware_test

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/middleware"
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
	h := middleware.OnMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})
	h(newTestSession(), newTestMessage("other-guild", testUserID, "hello"))
	if fired {
		t.Error("handler should not fire for a different guild")
	}
}

func TestOnMessage_BotSelf(t *testing.T) {
	fired := false
	h := middleware.OnMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})
	h(newTestSession(), newTestMessage(testGuildID, testBotID, "hello"))
	if fired {
		t.Error("handler should not fire for the bot's own messages")
	}
}

func TestOnMessage_Fires(t *testing.T) {
	fired := false
	h := middleware.OnMessage(testGuildID, func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})
	h(newTestSession(), newTestMessage(testGuildID, testUserID, "hello"))
	if !fired {
		t.Error("handler should fire for a real user message in the correct guild")
	}
}

func TestOnAnyMessage_BotSelf(t *testing.T) {
	fired := false
	h := middleware.OnAnyMessage(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})
	h(newTestSession(), newTestMessage(testGuildID, testBotID, "hello"))
	if fired {
		t.Error("handler should not fire for the bot's own messages")
	}
}

func TestOnAnyMessage_Fires(t *testing.T) {
	fired := false
	h := middleware.OnAnyMessage(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		fired = true
	})
	h(newTestSession(), newTestMessage("any-guild", testUserID, "hello"))
	if !fired {
		t.Error("handler should fire for a real user message regardless of guild")
	}
}
