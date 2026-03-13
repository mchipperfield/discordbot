package sd

import (
	"log/slog"
	"math/rand/v2"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/middleware"
	"github.com/mchipperfield/discordbot/voice"
)

var quotes = []string{
	"honestly",
	"sumptuous",
	"in my tenure",
	"the thiinnng is",
	"#cheers",
	"You know where it is",
	"it's not too bad actually",
	"i love the chocolate starfish",
	"i'm always prepared!",
	"Up the Spurs!",
	"SEND OUT!!!",
	"You are ENOUGH!",
	"I, do NOT fail!",
}

// Register wires all SD server handlers onto s for the given guildID.
// wakeUpSound is the pre-loaded opus audio for the wake-up gag; pass nil to disable.
func Register(s *discordgo.Session, guildID string, wakeUpSound [][]byte) {
	s.AddHandler(middleware.OnMessage(guildID, getQuote(quotes)))
	s.AddHandler(middleware.OnMessage(guildID, hungry()))
	s.AddHandler(middleware.OnMessage(guildID, wakeUp(wakeUpSound)))
}

// --- Handlers ----------------------------------------------------------------

func getQuote(qs []string) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isQuoteCommand(m.Content) {
			return
		}
		n := rand.IntN(len(qs) - 1)
		if _, err := s.ChannelMessageSend(m.ChannelID, qs[n]); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func hungry() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isHungry(m.Content) {
			return
		}
		if _, err := s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> Hungry? You know where the fridge is!"); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func wakeUp(opus [][]byte) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isTired(m.Content) {
			return
		}
		if err := voice.PlayAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
			slog.Info("could not play wake-up audio", "error", err, "user", m.Author.ID)
		}
	}
}

// --- Predicates --------------------------------------------------------------

func isQuoteCommand(content string) bool {
	return content == "!quote"
}

func isHungry(content string) bool {
	return strings.Contains(strings.ToLower(content), "hungry")
}

func isTired(content string) bool {
	return strings.Contains(strings.ToLower(content), "tired")
}
