package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
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
						slog.Info("failed to stop speaking", "error", err)
					}
					vc.Disconnect()
				}
			}
		}
	}
}

type CatImage struct {
	URL string `json:"url"`
}

func Kit(serverId string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	kitRegex, err := regexp.Compile(`\bkit\b`)
	if err != nil {
		slog.Info("failed to compile kit regex", "error", err)
		return nil
	}
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if kitRegex.MatchString(strings.ToLower(m.Content)) {
			resp, err := http.Get("https://api.thecatapi.com/v1/images/search")
			if err != nil {
				slog.Info("failed to get cat image", "error", err)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				slog.Info("failed to read cat image response body", "error", err)
				return
			}

			var catImages []CatImage
			if err := json.Unmarshal(body, &catImages); err != nil {
				slog.Info("failed to unmarshal cat image response", "error", err)
				return
			}
			if len(catImages) > 0 {
				_, err := s.ChannelMessageSend(m.ChannelID, catImages[0].URL)
				if err != nil {
					slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
				}
				s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> Who's a nice Kitty Kat?")
			}
		}
	}
}

func listen(serverId string, opus [][]byte) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	listenRegex, err := regexp.Compile(`\blisten\b`)
	if err != nil {
		slog.Info("failed to compile listen regex", "error", err)
		return nil
	}
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {

		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if listenRegex.MatchString(strings.ToLower(m.Content)) {
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
						slog.Info("failed to stop speaking", "error", err)
					}
					vc.Disconnect()
				}
			}
		}
	}
}

func AskGemini(service *ai.Service) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		switch i.ApplicationCommandData().Name {
		case "ask":
			// Defer the response, as the AI may take time to respond
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			})
			if err != nil {
				slog.Error("failed to defer interaction response", "error", err)
				return
			}

			// Access options in the order provided by the user.
			options := i.ApplicationCommandData().Options
			question := options[0].StringValue()

			response, err := service.Ask(question)
			if err != nil {
				slog.Error("failed to get response from AI", "error", err)
				response = "Sorry, I encountered an error trying to answer your question."
			}

			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &response,
			})
			if err != nil {
				slog.Error("failed to edit interaction response", "error", err)
			}
		default:
			slog.Error("unknown command", "command", i.ApplicationCommandData().Name)
		}
	}
}
