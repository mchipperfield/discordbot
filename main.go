package main

import (
	"flag"
	"log/slog"
	"math/rand/v2"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/peterbourgon/ff"

	"github.com/bwmarrin/discordgo"
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
}

// Server2985 is the guild id for the SD server.
const Server2985 = "133967162088069943"

func main() {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	var (
		token  = fs.String("bot_token", "", "bot authentication token")
		server = fs.String("server", Server2985, "server to listen on")
	)
	if err := ff.Parse(fs,
		os.Args[1:],
		ff.WithEnvVarNoPrefix(),
	); err != nil {
		slog.Info("failed to parse flags", "error", err)
		os.Exit(1)
	}

	session, err := discordgo.New("Bot " + *token)
	if err != nil {
		slog.Info("failed to create session", "error", err)
		os.Exit(1)
	}

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != *server {
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

		if m.Content == "!quote" {
			n := rand.IntN(len(quotes) - 1)
			_, err := s.ChannelMessageSend(m.ChannelID, quotes[n])
			if err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
		}
	})

	if err := session.Open(); err != nil {
		slog.Info("error opening websocket", "error", err)
		os.Exit(1)
	}
	slog.Info("websocket established")

	defer func() {
		if err := session.Close(); err != nil {
			slog.Info("error closing websocket", "error", err)
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stopChan

	slog.Info("signal received", "signal", sig)

}
