package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var americanToEnglish = map[string]string{
	"accessorize": "accessorise",
	"aluminum":    "aluminium",
	"analyze":     "analyse",
	"apologize":   "apologise",
	"authorize":   "authorise",
	"center":      "centre",
	"color":       "colour",
	"defense":     "defence",
	"enroll":      "enrol",
	"favorite":    "favourite",
	"flavor":      "flavour",
	"fulfill":     "fulfil",
	"honor":       "honour",
	"humor":       "humour",
	"judgment":    "judgement",
	"labor":       "labour",
	"license":     "licence",
	"liter":       "litre",
	"maneuver":    "manoeuvre",
	"meter":       "metre",
	"neighbor":    "neighbour",
	"offense":     "offence",
	"organize":    "organise",
	"practice":    "practise",
	"pretense":    "pretence",
	"realize":     "realise",
	"recognize":   "recognise",
	"rumor":       "rumour",
	"savior":      "saviour",
	"savor":       "savour",
	"skeptic":     "sceptic",
	"skillful":    "skilful",
	"theater":     "theatre",
	"tire":        "tyre",
	"traveler":    "traveller",
	"tumor":       "tumour",
	"vapor":       "vapour",
	"vigor":       "vigour",
	"willful":     "wilful",
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func cleanWord(word string) string {
	return nonAlphaNumeric.ReplaceAllString(word, "")
}

func americanSpellingPolice() func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		words := strings.Fields(m.Content)
		for _, word := range words {
			cleanedWord := cleanWord(strings.ToLower(word))
			if britishSpelling, ok := americanToEnglish[cleanedWord]; ok {
				reply := fmt.Sprintf("Did you mean %s?", britishSpelling)
				_, err := s.ChannelMessageSendReply(m.ChannelID, reply, m.Reference())
				if err != nil {
					slog.Error("failed to send spelling reply", "error", err)
				}
			}
		}
	}
}
