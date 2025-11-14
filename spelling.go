package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// LoadSpellingsFromURL fetches a text file from a URL and decodes it into a map.
// The text file should be in the format: uk_word:us_word
func LoadSpellingsFromURL(url string) (map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spellings from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch spellings, status code: %d", resp.StatusCode)
	}

	americanToBritish := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, " ")
		if len(parts) == 2 {
			ukWord, usWord := parts[0], parts[1]
			americanToBritish[usWord] = ukWord
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading spellings source: %w", err)
	}

	return americanToBritish, nil
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func cleanWord(word string) string {
	return nonAlphaNumeric.ReplaceAllString(word, "")
}

func americanSpellingPolice(americanToEnglish map[string]string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		var suggestions []string
		words := strings.Fields(m.Content)
		for _, word := range words {
			cleanedWord := cleanWord(strings.ToLower(word))
			if britishSpelling, ok := americanToEnglish[cleanedWord]; ok {
				suggestions = append(suggestions, fmt.Sprintf("`%s` -> `%s`", cleanedWord, britishSpelling))
			}
		}

		if len(suggestions) > 0 {
			reply := fmt.Sprintf("Did you mean: %s?", strings.Join(suggestions, ", "))
			_, err := s.ChannelMessageSendReply(m.ChannelID, reply, m.Reference())
			if err != nil {
				slog.Error("failed to send spelling reply", "error", err)
			}
		}
	}
}
