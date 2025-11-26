package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
)

const (
	ErrCodeClaimed  = "40008"
	ErrCodeExpired  = "40007"
	ErrCodeNotFound = "40014"
)

func getQuote(serverId string, quotes []string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Content == "!quote" {
			if m.GuildID != serverId {
				return
			}

			if m.Author.ID == s.State.User.ID {
				return
			}

			n := rand.IntN(len(quotes) - 1)
			_, err := s.ChannelMessageSend(m.ChannelID, quotes[n])
			if err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
		}
	}
}

func hungry(serverId string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.Contains(strings.ToLower(m.Content), "hungry") {
			user := m.Author.ID
			_, err := s.ChannelMessageSend(m.ChannelID, "<@"+user+"> Hungry? You know where the fridge is!")
			if err != nil {
				slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
			}
		}
	}
}

func wakeUp(serverId string, opus [][]byte) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {

		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.Contains(strings.ToLower(m.Content), "tired") {

			guild, err := s.State.Guild(m.GuildID)
			if err != nil {
				slog.Info("failed to get guild", "id", m.GuildID, "error", err)
			}
			for _, vs := range guild.VoiceStates {
				if vs.UserID == m.Author.ID {
					vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
					if err != nil {
						slog.Info("failed to join voice channel", "error", err, "id", vs.ChannelID)
					}

					if err := vc.Speaking(true); err != nil {
						slog.Info("failed to start speaking", "error", err)
					}
					time.Sleep(250 * time.Millisecond)
					for _, buf := range opus {
						vc.OpusSend <- buf
					}

					if err := vc.Speaking(false); err != nil {
						slog.Info("failed to stop speaking", "error", err)
					}
					vc.Disconnect()
				}
			}
		}
	}
}

type CatImage struct {
	URL string `json:"url"`
}

func Kit(serverId string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	kitRegex, err := regexp.Compile(`\bkit\b`)
	if err != nil {
		slog.Info("failed to compile kit regex", "error", err)
		return nil
	}
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if kitRegex.MatchString(strings.ToLower(m.Content)) {
			resp, err := http.Get("https://api.thecatapi.com/v1/images/search")
			if err != nil {
				slog.Info("failed to get cat image", "error", err)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				slog.Info("failed to read cat image response body", "error", err)
				return
			}

			var catImages []CatImage
			if err := json.Unmarshal(body, &catImages); err != nil {
				slog.Info("failed to unmarshal cat image response", "error", err)
				return
			}
			if len(catImages) > 0 {
				_, err := s.ChannelMessageSend(m.ChannelID, catImages[0].URL)
				if err != nil {
					slog.Info("failed to send message", "error", err, "channel", m.ChannelID)
				}
				s.ChannelMessageSend(m.ChannelID, "<@"+m.Author.ID+"> Who's a nice Kitty Kat?")
			}
		}
	}
}

func listen(serverId string, opus [][]byte) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	listenRegex, err := regexp.Compile(`\blisten\b`)
	if err != nil {
		slog.Info("failed to compile listen regex", "error", err)
		return nil
	}
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {

		if m.GuildID != serverId {
			return
		}

		if m.Author.ID == s.State.User.ID {
			return
		}

		if listenRegex.MatchString(strings.ToLower(m.Content)) {
			guild, err := s.State.Guild(m.GuildID)
			if err != nil {
				slog.Info("failed to get guild", "id", m.GuildID, "error", err)
			}
			for _, vs := range guild.VoiceStates {
				if vs.UserID == m.Author.ID {
					vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
					if err != nil {
						slog.Info("failed to join voice channel", "error", err, "id", vs.ChannelID)
					}

					if err := vc.Speaking(true); err != nil {
						slog.Info("failed to start speaking", "error", err)
					}
					time.Sleep(250 * time.Millisecond)
					for _, buf := range opus {
						vc.OpusSend <- buf
					}

					if err := vc.Speaking(false); err != nil {
						slog.Info("failed to stop speaking", "error", err)
					}
					vc.Disconnect()
				}
			}
		}
	}
}

func AskGemini(service *ai.Service) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {

		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		switch i.ApplicationCommandData().Name {
		case "ask":
			// Defer the response, as the AI may take time to respond
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			})
			if err != nil {
				slog.Error("failed to defer interaction response", "error", err)
				return
			}

			// Access options in the order provided by the user.
			options := i.ApplicationCommandData().Options
			question := options[0].StringValue()

			response, err := service.Ask(question)
			if err != nil {
				slog.Error("failed to get response from AI", "error", err)
				response = "Sorry, I encountered an error trying to answer your question."
			}

			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &response,
			})
			if err != nil {
				slog.Error("failed to edit interaction response", "error", err)
			}
		default:
			// This can be noisy if we have other interaction types.
			// slog.Error("unknown command", "command", i.ApplicationCommandData().Name)
		}
	}
}

var playerIDMutex = &sync.Mutex{}

func RegisterPlayer(playerIDFile string) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "register" {
			return
		}

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			slog.Error("failed to defer interaction response for register", "error", err)
			return
		}

		playerID := i.ApplicationCommandData().Options[0].StringValue()
		discordID := i.Member.User.ID

		// Validate Player ID with KingShot API
		// This calls the api to ensure the player ID exists and can be logged in, before registering it.
		loginResp, err := Login(playerID)
		if err != nil {
			slog.Error("failed to call login endpoint", "error", err)
			response := "Error validating player ID. Please try again later."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		if loginResp.Code != 0 {
			slog.Info("invalid player id", "player_id", playerID, "response_code", loginResp.Code, "response_message", loginResp.Message)
			response := "Invalid player ID provided."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		playerIDMutex.Lock()
		defer playerIDMutex.Unlock()

		// Read existing records
		records := make(map[string]string) // discordID -> playerID
		file, err := os.OpenFile(playerIDFile, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			slog.Error("failed to open or create player id file", "error", err, "file", playerIDFile)
			response := "Error registering player ID."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		csvRecords, err := reader.ReadAll()
		if err != nil && err != io.EOF {
			slog.Error("failed to read csv records", "error", err, "file", playerIDFile)
			response := "Error registering player ID."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		for _, record := range csvRecords {
			if len(record) == 2 {
				records[record[0]] = record[1]
			}
		}

		// Check for duplicate player ID
		for _, pID := range records {
			if pID == playerID {
				response := "This player ID has already been registered."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
				return
			}
		}

		// Add or update record
		records[discordID] = playerID

		// Write all records back to the file
		if err := file.Truncate(0); err != nil {
			slog.Error("failed to truncate player id file", "error", err)
		}
		if _, err := file.Seek(0, 0); err != nil {
			slog.Error("failed to seek in player id file", "error", err)
		}

		writer := csv.NewWriter(file)
		for dID, pID := range records {
			if err := writer.Write([]string{dID, pID}); err != nil {
				slog.Error("failed to write record to csv", "error", err)
			}
		}
		writer.Flush()

		if err := writer.Error(); err != nil {
			slog.Error("error writing csv file", "error", err, "file", playerIDFile)
			response := "Error writing player ID to file."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}
		slog.Info("user subscribed to bot", "player_id", playerID, "discord_id", discordID)

		response := "**Registration Successful!**\n**Your player ID *" + playerID + "* has been registered successfully!**"

		// After successful registration to the service, we will redeem any active codes for the user.
		// They may have previously, activated codes before registering. But this ensure they are upto date.

		var redemptionResults []string
		var codesToRemove []string

		for _, code := range ActiveCodes {
			redeemResp, err := RedeemGiftCode(playerID, code)
			if err != nil {
				slog.Error("failed to redeem gift code", "error", err, "code", code)
				redemptionResults = append(redemptionResults, fmt.Sprintf("`%s`: Error redeeming.", code))
				continue
			}
			slog.Info("redeem response", "code", code, "err_code", redeemResp.ErrCode, "message", redeemResp.Message)
			var resultMsg string
			switch string(redeemResp.ErrCode) {
			case ErrCodeClaimed:
				resultMsg = "Code Claimed!"
			case ErrCodeExpired:
				resultMsg = "Code Expired."
				ExpiredCodes = append(ExpiredCodes, code)
				codesToRemove = append(codesToRemove, code)
			case ErrCodeNotFound:
				resultMsg = "Code Not Found."
				codesToRemove = append(codesToRemove, code)
			default:
				resultMsg = redeemResp.Message
			}
			redemptionResults = append(redemptionResults, fmt.Sprintf("`%s`: %s", code, resultMsg))
		}

		// Remove expired or not-found codes from ActiveCodes
		if len(codesToRemove) > 0 {
			ActiveCodes = slices.DeleteFunc(ActiveCodes, func(code string) bool {
				return slices.Contains(codesToRemove, code)
			})
		}

		if len(redemptionResults) > 0 {
			response += "\n\n**Gift Code Redemption Results:**\n" + strings.Join(redemptionResults, "\n")
		}

		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &response,
		})
		if err != nil {
			slog.Error("failed to edit interaction response for register", "error", err)
		}
	}
}

func AddCode(playerIDFile string) func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "code" {
			return
		}

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		})
		if err != nil {
			slog.Error("failed to defer interaction response for code", "error", err)
			return
		}

		// --- Permission Check ---
		hasPermission := i.Member.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator

		if !hasPermission {
			guild, err := s.State.Guild(i.GuildID)
			if err != nil {
				slog.Error("failed to get guild for permission check", "error", err)
				response := "Error checking permissions."
				s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
				return
			}

			var adminRoleID string
			for _, role := range guild.Roles {
				if role.Name == "R4+" {
					adminRoleID = role.ID
					break
				}
			}

			if adminRoleID != "" {
				for _, roleID := range i.Member.Roles {
					if roleID == adminRoleID {
						hasPermission = true
						break
					}
				}
			}
		}

		if !hasPermission {
			response := "You do not have permission to use this command."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		// --- Command Logic ---
		playerIDMutex.Lock()
		defer playerIDMutex.Unlock()

		newCode := i.ApplicationCommandData().Options[0].StringValue()

		// Check if code already exists
		if slices.Contains(ActiveCodes, newCode) {
			response := fmt.Sprintf("Code `%s` is already active.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}
		if slices.Contains(ExpiredCodes, newCode) {
			response := fmt.Sprintf("Code `%s` has expired and cannot be re-added.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		// Read registered players
		file, err := os.Open(playerIDFile)
		if err != nil {
			response := fmt.Sprintf("Code `%s` has not been added, but we failed to open player file to redeem.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		csvRecords, err := reader.ReadAll()
		if err != nil {
			response := fmt.Sprintf("Code `%s` has not been added, as we failed to read player file to redeem.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		if len(csvRecords) == 0 {
			response := fmt.Sprintf("There are no registered players to redeem code %s.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		// --- Validate code with the first player ---
		firstPlayerID := csvRecords[0][1]

		_, err = Login(firstPlayerID)
		if err != nil {
			slog.Error("failed to call login endpoint before validating new code", "error", err)
		}

		redeemResp, err := RedeemGiftCode(firstPlayerID, newCode)
		if err != nil {
			response := fmt.Sprintf("Failed to validate code `%s` due to an error. The code has been removed.", newCode)
			slog.Error("failed to validate new code", "error", err, "code", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}

		// Test redemptrion result for first player.  If valid, proceed to redeem for all players.
		var firstResultMsg string
		switch string(redeemResp.ErrCode) {
		case ErrCodeClaimed:
			firstResultMsg = "Code Claimed!"
			return
		case ErrCodeExpired:
			firstResultMsg = "Expired."
			ExpiredCodes = append(ExpiredCodes, newCode)
			response := fmt.Sprintf("Code `%s` is expired.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		case ErrCodeNotFound:
			response := fmt.Sprintf("Code `%s` is not a valid gift-code.", newCode)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		default:
			firstResultMsg = redeemResp.Message
		}

		// Add code to active list if noit expired or invalid
		ActiveCodes = append(ActiveCodes, newCode)
		slog.Info("code added", "code", newCode)

		// --- Redeem for all players ---
		var redemptionResults []string
		redemptionResults = append(redemptionResults, fmt.Sprintf("Player `%s`: %s", firstPlayerID, firstResultMsg))
		time.Sleep(100 * time.Millisecond) // Delay after first request

		// Loop through the rest of the players
		for _, record := range csvRecords[1:] {
			if len(record) == 2 {
				playerID := record[1]

				_, err := Login(playerID)
				if err != nil {
					slog.Error("failed to call login endpoint before redeeming code for player", "error", err, "playerID", playerID)
				}

				redeemResp, err := RedeemGiftCode(playerID, newCode)

				var resultMsg string
				if err != nil {
					slog.Error("failed to redeem gift code for player", "error", err, "code", newCode, "playerID", playerID)
					resultMsg = "Error redeeming."
				} else {
					switch string(redeemResp.ErrCode) {
					case ErrCodeClaimed:
						resultMsg = "Code Claimed!"
					case ErrCodeExpired:
						resultMsg = "Code Expired."
					case ErrCodeNotFound:
						resultMsg = "Code not found."
					default:
						resultMsg = redeemResp.Message
					}
				}

				redemptionResults = append(redemptionResults, fmt.Sprintf("Player `%s`: %s", playerID, resultMsg))

				// Small delay to avoid rate-limiting
				time.Sleep(100 * time.Millisecond)
			}
		}

		response := fmt.Sprintf("Code `%s` has been added to the active list.\n\n**Redemption Results for %d players:**\n%s", newCode, len(csvRecords), strings.Join(redemptionResults, "\n"))
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
	}
}
