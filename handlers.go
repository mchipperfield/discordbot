package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
)

func getQuote(quotes []string) MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isQuoteCommand(m.Content) {
			return
		}
		n := rand.IntN(len(quotes) - 1)
		if _, err := s.ChannelMessageSend(m.ChannelID, quotes[n]); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func hungry() MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isHungry(m.Content) {
			return
		}
		if _, err := s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> Hungry? You know where the fridge is!"); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func wakeUp(opus [][]byte) MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isTired(m.Content) {
			return
		}
		if err := playAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
			slog.Info("could not play wake-up audio", "error", err, "user", m.Author.ID)
		}
	}
}

type CatImage struct {
	URL string `json:"url"`
}

func Kit() MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isKit(m.Content) {
			return
		}
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
			if _, err := s.ChannelMessageSend(m.ChannelID, catImages[0].URL); err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
			s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> Who's a nice Kitty Kat?")
		}
	}
}

func Blondie() MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isFullSend(m.Content) {
			return
		}
		if _, err := s.ChannelMessageSendReply(m.ChannelID, "WWBD?", m.Reference()); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func listen(opus [][]byte) MessageHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isListen(m.Content) {
			return
		}
		if err := playAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
			slog.Info("could not play listen audio", "error", err, "user", m.Author.ID)
		}
	}
}

func AskGemini(service *ai.Service) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if i.ApplicationCommandData().Name != "ask" {
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		}); err != nil {
			slog.Error("failed to defer interaction response", "error", err)
			return
		}

		question := i.ApplicationCommandData().Options[0].StringValue()
		response, err := service.Ask(question)
		if err != nil {
			slog.Error("failed to get response from AI", "error", err)
			response = "Sorry, I encountered an error trying to answer your question."
		}

		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response}); err != nil {
			slog.Error("failed to edit interaction response", "error", err)
		}
	}
}
