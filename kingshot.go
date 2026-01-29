package main

import (
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/time/rate"
)

const (
	LoginUrl  string = "https://kingshot-giftcode.centurygame.com/api/player"
	RedeemUrl string = "https://kingshot-giftcode.centurygame.com/api/gift_code"
	Key       string = "mN4!pQs6JrYwV9"
)

// Error codes from the KingShot API
const (
	ErrCodeSuccess  = "20000"
	ErrCodeClaimed  = "40008"
	ErrCodeExpired  = "40007"
	ErrCodeNotFound = "40014"
	ErrCodeLogin    = "40009"
)
const r4RoleId = "1432032487021875373" // RoleId for permission check

var ActiveCodes = []string{}

var ExpiredCodes []string

var playerIDMutex = &sync.Mutex{}

type transport struct {
	limiter *rate.Limiter
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	err := t.limiter.Wait(req.Context())
	if err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(req)
}

var httpClient = http.Client{
	Transport: &transport{
		limiter: rate.NewLimiter(rate.Every(2*time.Second), 1),
	},
}

func GiftCodeCommandHandler(playerIDFile, giftCodeChannelID string) func(s *discordgo.Session, r *discordgo.Ready) {
	return func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("Registering gift code commands and handlers")
		commands := []*discordgo.ApplicationCommand{
			{
				Name:        "register",
				Description: "Register your KingShot player ID",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "player-id",
						Description: "Your KingShot player ID",
						Required:    true,
					},
				},
			},
			{
				Name:        "code",
				Description: "Adds a new gift code for redemption.",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "code",
						Description: "The gift code to add.",
						Required:    true,
					},
				},
			},
		}

		for _, v := range commands {
			_, err := s.ApplicationCommandCreate(s.State.User.ID, "1423406563850190850", v)
			if err != nil {
				slog.Error("cannot create command", "command", v.Name, "error", err)
			}
		}
		s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if i.Type != discordgo.InteractionApplicationCommand {
				return
			}
			switch i.ApplicationCommandData().Name {
			case "register":
				registerPlayer(s, i, playerIDFile)
			case "code":
				addCode(s, i, playerIDFile)
			}
		})
		s.AddHandler(messageHandler(playerIDFile, giftCodeChannelID))
	}
}

func registerPlayer(s *discordgo.Session, i *discordgo.InteractionCreate, playerIDFile string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		slog.Error("failed to defer interaction response for register", "error", err)
		return
	}

	playerID := i.ApplicationCommandData().Options[0].StringValue()
	discordID := i.Member.User.ID

	// Validate Player ID with KingShot API
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
	playerToDiscord := make(map[string]string)

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
			playerToDiscord[record[0]] = record[1]
		}
	}

	// Check if player ID is already registered.
	if existingDiscordID, ok := playerToDiscord[playerID]; ok {
		if existingDiscordID == discordID {
			response := "This player ID is already registered to your Discord account."
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
			return
		}
		response := "This player ID is already registered to another Discord account."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
		return
	}

	// Add the new player ID.
	playerToDiscord[playerID] = discordID

	// Rewrite the entire file with updated records.
	if err := file.Truncate(0); err != nil {
		slog.Error("failed to truncate player id file", "error", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		slog.Error("failed to seek player id file", "error", err)
	}

	writer := csv.NewWriter(file)
	for pID, dID := range playerToDiscord {
		if err := writer.Write([]string{pID, dID}); err != nil {
			slog.Error("failed to write record to csv", "error", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		slog.Error("failed to flush csv writer", "error", err)
		response := "Error registering player ID."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
		return
	}

	slog.Info("user subscribed to bot", "player_id", playerID, "discord_id", discordID)

	response := "**Registration Successful!**\n**Your player ID *" + playerID + "* has been registered successfully!**"

	// After successful registration to the service, we will redeem any active codes for the user.
	var redemptionResults []string
	var codesToRemove []string

	for _, code := range ActiveCodes {
		redeemResp, err := RedeemGiftCode(playerID, code)
		if err != nil {
			slog.Error("failed to redeem gift code after registration", "error", err, "code", code, "player_id", playerID)
			redemptionResults = append(redemptionResults, fmt.Sprintf("`%s`: Error redeeming code.", code))
			continue
		}
		slog.Info("redeem response", "code", code, "err_code", redeemResp.ErrCode, "message", redeemResp.Message, "player_id", playerID)
		var resultMsg string
		switch string(redeemResp.ErrCode) {
		case ErrCodeSuccess:
			resultMsg = "Successfully redeemed!"
		case ErrCodeClaimed:
			resultMsg = "Already claimed."
		case ErrCodeExpired, ErrCodeNotFound:
			resultMsg = "Code expired or not found."
			codesToRemove = append(codesToRemove, code)
		case ErrCodeLogin:
			resultMsg = "Unable to login"
		default:
			resultMsg = "Failed to redeem code."
		}
		redemptionResults = append(redemptionResults, fmt.Sprintf("`%s`: %s", code, resultMsg))
	}

	// Remove expired or not-found codes from ActiveCodes
	if len(codesToRemove) > 0 {
		ActiveCodes = slices.DeleteFunc(ActiveCodes, func(c string) bool {
			return slices.Contains(codesToRemove, c)
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

func addCode(s *discordgo.Session, i *discordgo.InteractionCreate, playerIDFile string) {
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
		for _, roleID := range i.Member.Roles {
			if roleID == r4RoleId {
				hasPermission = true
				break
			}
		}
	}

	if !hasPermission {
		response := "You do not have permission to use this command."
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &response})
		return
	}

	// --- Command Logic ---
	newCode := i.ApplicationCommandData().Options[0].StringValue()

	response := processNewCode(playerIDFile, newCode)
	if len(response) > 1900 {
		// Split the response into chunks
		var chunks []string
		for len(response) > 0 {
			runeLimit := 1900
			if len(response) < runeLimit {
				runeLimit = len(response)
			}
			// Ensure we don't split in the middle of a line
			lastNewline := strings.LastIndex(response[:runeLimit], "\n")
			if lastNewline == -1 {
				lastNewline = runeLimit
			}
			chunks = append(chunks, response[:lastNewline])
			response = response[lastNewline:]
			if len(response) > 0 && response[0] == '\n' {
				response = response[1:]
			}
		}
		for _, chunk := range chunks {
			s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: chunk,
			})
		}
	} else {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: response,
		})
	}
}

func messageHandler(playerIDFile, channelID string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Regex to find a gift code from the message content
	codeRegex := regexp.MustCompile(`Gift Code: ([A-Z0-9]+)`)

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore messages that are not from a bot, not in the specified channel, or not a default message type.
		// This is indicative of a message from a "followed" channel.
		if !m.Author.Bot || m.ChannelID != channelID || m.Type != discordgo.MessageTypeDefault {
			return
		}

		slog.Info("followed channel message received", "author", m.Author.Username)

		matches := codeRegex.FindStringSubmatch(m.Content)
		if len(matches) < 2 {
			slog.Info("no gift code found in message content")
			return
		}

		newCode := matches[1]
		slog.Info("extracted gift code", "code", newCode)

		// Send a thinking message
		thinkingMsg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Processing new gift code: `%s`...", newCode))
		if err != nil {
			slog.Error("failed to send thinking message", "error", err)
		}

		result := processNewCode(playerIDFile, newCode)

		// Edit the original message with the result
		if thinkingMsg != nil {
			_, err := s.ChannelMessageEdit(thinkingMsg.ChannelID, thinkingMsg.ID, result)
			if err != nil {
				slog.Error("failed to edit message with result", "error", err)
				// Fallback to sending a new message if edit fails
				s.ChannelMessageSend(m.ChannelID, result)
			}
		} else {
			s.ChannelMessageSend(m.ChannelID, result)
		}
	}
}

// processNewCode handles the logic of validating and redeeming a new gift code.
func processNewCode(playerIDFile, newCode string) string {
	playerIDMutex.Lock()
	defer playerIDMutex.Unlock()

	// Check if code already exists
	if slices.Contains(ActiveCodes, newCode) {
		return fmt.Sprintf("Code `%s` is already active.", newCode)
	}
	if slices.Contains(ExpiredCodes, newCode) {
		return fmt.Sprintf("Code `%s` has expired and cannot be re-added.", newCode)
	}

	// Read registered players
	file, err := os.Open(playerIDFile)
	if err != nil {
		return fmt.Sprintf("Code `%s` has not been added, as we failed to open player file.", newCode)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	csvRecords, err := reader.ReadAll()
	if err != nil {
		return fmt.Sprintf("Code `%s` has not been added, as we failed to read player file to redeem.", newCode)
	}

	if len(csvRecords) == 0 {
		// Still add the code to the active list for future registrations.
		ActiveCodes = append(ActiveCodes, newCode)
		slog.Info("code added", "code", newCode)
		return fmt.Sprintf("There are no registered players, but code `%s` has been added to the active list.", newCode)
	}

	// --- Validate code with the first player ---
	firstPlayerID := csvRecords[0][0]

	_, err = Login(firstPlayerID)
	if err != nil {
		slog.Error("failed to call login endpoint before validating new code", "error", err)
		return fmt.Sprintf("Failed to validate code `%s` due to an error. The code has not been added.", newCode)
	}

	redeemResp, err := RedeemGiftCode(firstPlayerID, newCode)
	if err != nil {
		slog.Error("failed to validate new code", "error", err, "code", newCode)
		return fmt.Sprintf("Failed to validate code `%s` due to an error. The code has not been added.", newCode)
	}

	// Test redemption result for first player.
	var firstResultMsg string
	slog.Info("redeem response", "code", newCode, "err_code", redeemResp.ErrCode, "message", redeemResp.Message, "player_id", firstPlayerID)
	switch string(redeemResp.ErrCode) {
	case ErrCodeSuccess:
		firstResultMsg = "Successfully redeemed!"
	case ErrCodeClaimed:
		firstResultMsg = "Already claimed."
	case ErrCodeExpired:
		ExpiredCodes = append(ExpiredCodes, newCode)
		return fmt.Sprintf("Code `%s` has expired and was not added.", newCode)
	case ErrCodeNotFound:
		return fmt.Sprintf("Code `%s` is not valid and was not added.", newCode)
	case ErrCodeLogin:
		return fmt.Sprintf("Code `%s` could not be validated - unable to login.", newCode)
	default:
		firstResultMsg = "Failed to redeem code."
	}

	// Add code to active list if not expired or invalid
	ActiveCodes = append(ActiveCodes, newCode)
	slog.Info("code added", "code", newCode)

	// --- Redeem for all players ---
	var redemptionResults []string
	redemptionResults = append(redemptionResults, fmt.Sprintf("Player `%s`: %s", firstPlayerID, firstResultMsg))
	time.Sleep(100 * time.Millisecond) // Delay after first request

	results := make(chan string, len(csvRecords)-1)

	var wg sync.WaitGroup
	wg.Add(len(csvRecords) - 1)
	for _, record := range csvRecords[1:] {
		if len(record) != 2 {
			wg.Done()
			continue
		}
		go func() {
			defer wg.Done()

			playerID := record[0]
			var resultMsg string

			// Small delay to avoid rate limiting
			//time.Sleep(100 * time.Millisecond)
			_, err = Login(playerID)
			if err != nil {
				slog.Error("failed to call login endpoint", "error", err)
				resultMsg = "Failed to login."
			} else {
				redeemResp, err := RedeemGiftCode(playerID, newCode)
				if err != nil {
					slog.Error("failed to redeem gift code in worker", "error", err, "code", newCode, "player_id", playerID)
					resultMsg = "Error redeeming code."
				} else {
					slog.Info("redeem response", "code", newCode, "err_code", redeemResp.ErrCode, "message", redeemResp.Message, "player_id", playerID)
					switch string(redeemResp.ErrCode) {
					case ErrCodeSuccess:
						resultMsg = "Successfully redeemed!"
					case ErrCodeClaimed:
						resultMsg = "Already claimed."
					case ErrCodeLogin:
						resultMsg = "Unable to login"
					default:
						resultMsg = "Failed to redeem."
					}
				}
			}
			results <- fmt.Sprintf("Player `%s`: %s", playerID, resultMsg)

		}()
	}

	wg.Wait()
	close(results)

	// Collect results
	for result := range results {
		redemptionResults = append(redemptionResults, result)
	}

	return fmt.Sprintf("Code `%s` has been added to the active list.\n\n**Redemption Results for %d players:**\n%s", newCode, len(csvRecords), strings.Join(redemptionResults, "\n"))
}

// EncodePayload encodes the data map into a JSON string with an MD5 signature
// This is the required format for the KingShot API.
func EncodePayload(data map[string]string) (string, error) {
	values := url.Values{}

	// 2. Data Encoding/Serialization (The Python equivalent of f"{key}={...}")
	for key, value := range data {
		values.Set(key, value)
	}

	// Signature Generation (MD5 Hashing)
	// Combine encoded data and secret: "{encoded_data}{secret}"
	dataToHash := values.Encode() + Key

	// Calculate MD5 hash
	hasher := md5.New()
	hasher.Write([]byte(dataToHash))

	// Convert the hash bytes to a hex string
	signature := hex.EncodeToString(hasher.Sum(nil))

	// Return the original data, with signature
	data["sign"] = signature

	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// ErrCode is a custom type to handle err_code being a number or a string.
type ErrCode string

// UnmarshalJSON implements the json.Unmarshaler interface.
func (e *ErrCode) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a string.
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*e = ErrCode(s)
		return nil
	}

	// If it's not a string, try to unmarshal as a number.
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*e = ErrCode(strconv.Itoa(i))
		return nil
	}

	return fmt.Errorf("err_code is not a string or a number: %s", data)
}

// LoginResponse represents the response from the login endpoint.
// ErrCode is a custom type to handle err_code being a number or a string.
type LoginResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	Data    any     `json:"data"`
	ErrCode ErrCode `json:"err_code"`
}

func Login(fid string) (*LoginResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"time": fmt.Sprintf("%d", time.Now().Unix()),
	}

	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Post(LoginUrl, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w", err)
	}
	return &loginResp, nil
}

type RedeemResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	ErrCode ErrCode `json:"err_code"`
}

func RedeemGiftCode(fid, cdk string) (*RedeemResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"cdk":  cdk,
		"time": fmt.Sprintf("%d", time.Now().Unix()),
	}

	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Post(RedeemUrl, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var redeemResponse RedeemResponse
	if err := json.NewDecoder(resp.Body).Decode(&redeemResponse); err != nil {
		return nil, fmt.Errorf("failed to decode redemption response: %w", err)
	}

	return &redeemResponse, nil
}
