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
	americanToBritish["bitch"] = "bish"
	americanToBritish["bitches"] = "bishes"

	return americanToBritish, nil
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func cleanWord(word string) string {
	return nonAlphaNumeric.ReplaceAllString(word, "")
}

// buildSpellingSuggestions returns a list of formatted suggestion strings for any
// American spellings found in the message, keyed against the americanToEnglish map.
func buildSpellingSuggestions(message string, americanToEnglish map[string]string) []string {
	var suggestions []string
	for _, word := range strings.Fields(message) {
		cleanedWord := cleanWord(strings.ToLower(word))
		if britishSpelling, ok := americanToEnglish[cleanedWord]; ok {
			suggestions = append(suggestions, fmt.Sprintf("`%s` -> `%s`", cleanedWord, britishSpelling))
		}
	}
	return suggestions
}

func americanSpellingPolice(americanToEnglish map[string]string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		suggestions := buildSpellingSuggestions(m.Content, americanToEnglish)
		if len(suggestions) > 0 {
			reply := fmt.Sprintf("Did you mean: %s?", strings.Join(suggestions, ", "))
			_, err := s.ChannelMessageSendReply(m.ChannelID, reply, m.Reference())
			if err != nil {
				slog.Error("failed to send spelling reply", "error", err)
			}
		}
	}
}
