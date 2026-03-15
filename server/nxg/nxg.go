package nxg

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
	"github.com/mchipperfield/discordbot/middleware"
	"github.com/mchipperfield/discordbot/voice"
)

// knownUsers maps Discord user IDs to friendly names for use in handlers.
var knownUsers = []string{"845223962052788224"} // Kinslayer discord ID

var (
	kitRegex      = regexp.MustCompile(`\bkit\b`)
	fullSendRegex = regexp.MustCompile(`\bfull send\b`)
	listenRegex   = regexp.MustCompile(`\blisten\b`)
)

// Register wires all NXG server handlers onto s for the given guildID.
// listenSound is the pre-loaded opus audio for the listen gag; pass nil to disable.
// aiSvc is optional — pass nil to skip the /ask command handler.
func Register(s *discordgo.Session, guildID string, listenSound [][]byte, aiSvc *ai.Service) {
	s.AddHandler(middleware.OnMessage(guildID, kit()))
	s.AddHandler(middleware.OnMessage(guildID, blondie()))
	s.AddHandler(middleware.OnMessage(guildID, listenAudio(listenSound)))
	s.AddHandler(middleware.OnMessage(guildID, dadJoke()))
	if aiSvc != nil {
		s.AddHandler(askGemini(aiSvc))
	}
}

// --- Handlers ----------------------------------------------------------------

type catImage struct {
	URL string `json:"url"`
}

func kit() func(*discordgo.Session, *discordgo.MessageCreate) {
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
		var catImages []catImage
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

func blondie() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isFullSend(m.Content) {
			return
		}
		if _, err := s.ChannelMessageSendReply(m.ChannelID, "WWBD?", m.Reference()); err != nil {
			slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
		}
	}
}

func listenAudio(opus [][]byte) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isListen(m.Content) {
			return
		}
		if err := voice.PlayAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
			slog.Info("could not play listen audio", "error", err, "user", m.Author.ID)
		}
	}
}

type dadJokeResponse struct {
	Joke string `json:"joke"`
}

func dadJoke() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !mentionsKnownUser(m) {
			return
		}
		req, _ := http.NewRequest("GET", "https://icanhazdadjoke.com/", nil)
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slog.Info("failed to fetch dad joke", "error", err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Info("failed to read dad joke response", "error", err)
			return
		}
		var joke dadJokeResponse
		if err := json.Unmarshal(body, &joke); err != nil {
			slog.Info("failed to unmarshal dad joke", "error", err)
			return
		}
		if _, err := s.ChannelMessageSend(m.ChannelID, joke.Joke); err != nil {
			slog.Info("failed to send dad joke", "error", err, "channel", m.ChannelID)
		}
	}
}

func askGemini(service *ai.Service) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

// --- Predicates --------------------------------------------------------------

func isKit(content string) bool {
	return kitRegex.MatchString(strings.ToLower(content))
}

func isFullSend(content string) bool {
	return fullSendRegex.MatchString(strings.ToLower(content))
}

func isListen(content string) bool {
	return listenRegex.MatchString(strings.ToLower(content))
}

func mentionsKnownUser(m *discordgo.MessageCreate) bool {
	for _, u := range m.Mentions {
		if slices.Contains(knownUsers, u.ID) {
			return true
		}
	}
	return false
}
