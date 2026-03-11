package voice_test

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/voice"
)

func TestPlayAudioToUser_GuildNotFound(t *testing.T) {
	s := &discordgo.Session{State: discordgo.NewState()}

	err := voice.PlayAudioToUser(s, "nonexistent-guild", "user1", nil)
	if err == nil {
		t.Fatal("expected error when guild is not in state, got nil")
	}
}

func TestPlayAudioToUser_UserNotInVoice(t *testing.T) {
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{
		ID:          "guild1",
		VoiceStates: []*discordgo.VoiceState{},
	}); err != nil {
		t.Fatal(err)
	}

	s := &discordgo.Session{State: state}
	err := voice.PlayAudioToUser(s, "guild1", "user1", nil)
	if err == nil {
		t.Fatal("expected error when user is not in any voice channel, got nil")
	}
}

func TestPlayAudioToUser_OtherUserInVoiceButNotTarget(t *testing.T) {
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{
		ID: "guild1",
		VoiceStates: []*discordgo.VoiceState{
			{UserID: "other-user", ChannelID: "channel1"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	s := &discordgo.Session{State: state}
	err := voice.PlayAudioToUser(s, "guild1", "target-user", nil)
	if err == nil {
		t.Fatal("expected error when target user is not in voice, got nil")
	}
}
