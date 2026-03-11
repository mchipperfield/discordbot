package main

import "github.com/bwmarrin/discordgo"

type MessageHandler func(*discordgo.Session, *discordgo.MessageCreate)

// onMessage wraps h so it only fires for real user messages in guildID.
func onMessage(guildID string, h MessageHandler) func(*discordgo.Session, *discordgo.MessageCreate) {
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

// onAnyMessage wraps h so it fires for real user messages in any guild.
func onAnyMessage(h MessageHandler) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		h(s, m)
	}
}
