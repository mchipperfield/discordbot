package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestIsAllCaps(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"HELLO", true},
		{"hello", false},
		{"Hello", false},
		{"HELLO!!!", true},
		{"STOP 123", true},
		{"HELLO WORLD", true},
		{"Hello World", false},
		{"", false},
		{"!!!", false},
		{"123", false},
		{"123ABC", true},
		{"abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isAllCaps(tt.input)
			if got != tt.want {
				t.Errorf("isAllCaps(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func newShoutingSession(t *testing.T, channelID string, sent *string) *discordgo.Session {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				*sent = body.Content
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","channel_id":"` + channelID + `","content":""}`))
	}))
	t.Cleanup(srv.Close)

	origDiscord := discordgo.EndpointDiscord
	origAPI := discordgo.EndpointAPI
	origChannels := discordgo.EndpointChannels
	origMessages := discordgo.EndpointChannelMessages
	t.Cleanup(func() {
		discordgo.EndpointDiscord = origDiscord
		discordgo.EndpointAPI = origAPI
		discordgo.EndpointChannels = origChannels
		discordgo.EndpointChannelMessages = origMessages
	})

	discordgo.EndpointDiscord = srv.URL + "/"
	discordgo.EndpointAPI = discordgo.EndpointDiscord + "api/v10/"
	discordgo.EndpointChannels = discordgo.EndpointAPI + "channels/"
	discordgo.EndpointChannelMessages = func(cID string) string {
		return discordgo.EndpointChannels + cID + "/messages"
	}

	s, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	s.State = discordgo.NewState()
	s.State.User = &discordgo.User{ID: "bot-id"}
	return s
}

func newShoutingMessage(authorID, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   content,
			Author:    &discordgo.User{ID: authorID, Username: "tester"},
		},
	}
}

func TestShoutingPolice_WrongUser(t *testing.T) {
	h := shoutingPolice("target-user")
	// Passing nil session: handler must return early without calling any session method.
	h(nil, newShoutingMessage("other-user", "HELLO"))
}

func TestShoutingPolice_NotAllCaps(t *testing.T) {
	h := shoutingPolice("target-user")
	h(nil, newShoutingMessage("target-user", "hello"))
}

func TestShoutingPolice_SendsMessage(t *testing.T) {
	var sent string
	s := newShoutingSession(t, "chan-1", &sent)

	h := shoutingPolice("target-user")
	m := newShoutingMessage("target-user", "HELLO WORLD")
	h(s, m)

	want := "Hey " + m.Author.Mention() + ", stop shouting!"
	if sent != want {
		t.Errorf("sent message = %q, want %q", sent, want)
	}
}
