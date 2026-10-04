package alliance

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/middleware"
)

func Register(s *discordgo.Session, guildID string, userID string) {
	s.AddHandler(middleware.OnMessage(guildID, kat(userID)))
}

func kat(userID string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Implement the kat message handler logic here
		if m.Author.ID == userID {
			_, err := s.ChannelMessageSendReply(m.ChannelID, "I'm a Spring Chicken", m.Reference())
			if err != nil {
				slog.Error("failed to send message", "error", err) // Handle the error if needed
			}
		}
	}
}
