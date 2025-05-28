package main

import (
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func getQuote(serverId string, quotes []string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Content == "!quote" {
			if m.GuildID != serverId {
				return
			}

			if m.Author.ID == s.State.User.ID {
				return
			}

			n := rand.IntN(len(quotes) - 1)
			_, err := s.ChannelMessageSend(m.ChannelID, quotes[n])
			if err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
		}
	}
}

func hungry(serverId string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.Contains(strings.ToLower(m.Content), "hungry") {
			user := m.Author.ID
			_, err := s.ChannelMessageSend(m.ChannelID, "<@"+user+"> Hungry? You know where the fridge is!")
			if err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
		}
	}
}

func wakeUp(serverId string, opus [][]byte) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {

		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.Contains(strings.ToLower(m.Content), "tired") {

			guild, err := s.State.Guild(m.GuildID)
			if err != nil {
				slog.Info("failed to get guild", "id", m.GuildID, "error", err)
			}
			for _, vs := range guild.VoiceStates {
				if vs.UserID == m.Author.ID {
					vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
					if err != nil {
						slog.Info("failed to join voice channel", "error", err, "id", vs.ChannelID)
					}

					if err := vc.Speaking(true); err != nil {
						slog.Info("failed to start speaking", "error", err)
					}
					time.Sleep(250 * time.Millisecond)
					for _, buf := range opus {
						vc.OpusSend <- buf
					}

					if err := vc.Speaking(false); err != nil {
						slog.Info("failed to start speaking", "error", err)
					}
					vc.Disconnect()
				}
			}
		}
	}
}
