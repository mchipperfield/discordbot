package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"unicode"

	"github.com/bwmarrin/discordgo"
)

const shoutingTargetUserID = "1424459357361406074"

var nonAlpha = regexp.MustCompile(`[^a-zA-Z]+`)

func isAllCaps(message string) bool {
	letters := nonAlpha.ReplaceAllString(message, "")
	if len(letters) == 0 {
		return false
	}
	for _, r := range letters {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func shoutingPolice(userID string) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID != userID {
			return
		}
		if !isAllCaps(m.Content) {
			return
		}
		_, err := s.ChannelMessageSendReply(m.ChannelID, fmt.Sprintf("Hey %s, stop shouting!", m.Author.Mention()), m.Reference())
		if err != nil {
			slog.Error("failed to send shouting reply", "error", err)
		}
	}
}
