package nxg

import (
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func yana() func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if !isYana(m.Content) {
			return
		}
		f, err := os.Open("yana.jpg")
		if err != nil {
			slog.Info("failed to open yana image", "error", err)
			return
		}
		defer f.Close()

		if _, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: "I'll make you my bish",
			Files: []*discordgo.File{{
				Name:   "yana.jpg",
				Reader: f,
			}},
			Reference: m.Reference(),
		}); err != nil {
			slog.Info("failed to send yana image", "error", err, "channel", m.ChannelID)
		}
	}
}

var yanaRegex = regexp.MustCompile(`\bbish\b`)

func isYana(content string) bool {
	return yanaRegex.MatchString(strings.ToLower(content))
}
