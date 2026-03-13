package middleware

import "github.com/bwmarrin/discordgo"

// OnMessage wraps h so it only fires for real user messages in guildID.
func OnMessage(guildID string, h func(*discordgo.Session, *discordgo.MessageCreate)) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != guildID {
			return
		}
		if m.Author.ID == s.State.User.ID {
			return
		}
		h(s, m)
	}
}

// OnAnyMessage wraps h so it fires for real user messages in any guild.
func OnAnyMessage(h func(*discordgo.Session, *discordgo.MessageCreate)) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		h(s, m)
	}
}
