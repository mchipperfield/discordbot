package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/kingshot"
)

// Register wires KingShot Discord handlers onto the given session.
func Register(s *discordgo.Session, ks *kingshot.KingShot, giftCodeChannelID string) {
	s.AddHandler(ks.InteractionHandler())
	s.AddHandler(ks.MessageHandler(giftCodeChannelID))
}
