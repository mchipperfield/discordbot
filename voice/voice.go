package voice

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
)

const voiceStartupDelay = 250 * time.Millisecond

// PlayAudioToUser joins the voice channel of userID in guildID, plays opusFrames,
// then disconnects. Returns an error if the guild is not in state or the user
// is not currently in any voice channel.
func PlayAudioToUser(s *discordgo.Session, guildID, userID string, opusFrames [][]byte) error {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return fmt.Errorf("guild %s not found in state: %w", guildID, err)
	}

	for _, vs := range guild.VoiceStates {
		if vs.UserID != userID {
			continue
		}
		vc, err := s.ChannelVoiceJoin(guildID, vs.ChannelID, false, false)
		if err != nil {
			return fmt.Errorf("failed to join voice channel %s: %w", vs.ChannelID, err)
		}
		if err := vc.Speaking(true); err != nil {
			slog.Info("failed to start speaking", "error", err)
		}
		time.Sleep(voiceStartupDelay)
		for _, buf := range opusFrames {
			vc.OpusSend <- buf
		}
		if err := vc.Speaking(false); err != nil {
			slog.Info("failed to stop speaking", "error", err)
		}
		vc.Disconnect()
		return nil
	}

	return fmt.Errorf("user %s is not in any voice channel", userID)
}
