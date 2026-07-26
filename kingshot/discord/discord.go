package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/kingshot"
)

// Register wires KingShot Discord handlers onto the given session at startup.
// giftCodeChannelID is the channel monitored for bot-posted gift code messages.
func Register(s *discordgo.Session, ks *kingshot.GiftCodeService, giftCodeChannelID string) {
	s.AddHandler(ks.InteractionHandler())
	s.AddHandler(ks.MessageHandler(giftCodeChannelID))
}
