package kingshot

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	csvstore "github.com/mchipperfield/discordbot/kingshot/storage/csv"
	"golang.org/x/time/rate"
)

// API endpoints and signing key for the KingShot gift code service.
const (
	defaultLoginURL  = "https://kingshot-giftcode.centurygame.com/api/player"
	defaultRedeemURL = "https://kingshot-giftcode.centurygame.com/api/gift_code"
	Key              = "mN4!pQs6JrYwV9"
)

// Error codes returned by the KingShot API.
const (
	ErrCodeSuccess      = "20000"
	ErrCodeClaimed      = "40008"
	ErrCodeExpired      = "40007"
	ErrCodeNotFound     = "40014"
	ErrCodeLogin        = "40009"
	ErrCodeLimitReached = "40005"
)

// r4RoleId is the Discord role that is allowed to add gift codes.
const r4RoleId = "1432032487021875373"

// discordMaxMessageLen is the safe character limit for a single Discord message.
const discordMaxMessageLen = 1900

// transport is a rate-limited http.RoundTripper.
type transport struct {
	limiter *rate.Limiter
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return http.DefaultTransport.RoundTrip(req)
}

// KingShot manages gift code state and handles all KingShot-related Discord
// interactions. All mutable state is owned here; no package-level globals.
type KingShot struct {
	mu           sync.Mutex
	activeCodes  []string
	expiredCodes []string
	playerIDFile string
	store        PlayerStore
	client       *http.Client
	loginURL     string
	redeemURL    string
}

// NewKingShot returns a KingShot ready for production use. Any activeCodes
// provided are pre-loaded as already-active codes.
func NewKingShot(playerIDFile string, activeCodes ...string) *KingShot {
	return NewKingShotWithStore(csvstore.New(playerIDFile), activeCodes...)
}

// NewKingShotWithStore returns a KingShot backed by store.
func NewKingShotWithStore(store PlayerStore, activeCodes ...string) *KingShot {
	return &KingShot{
		activeCodes: activeCodes,
		store:       store,
		loginURL:    defaultLoginURL,
		redeemURL:   defaultRedeemURL,
		client: &http.Client{
			Transport: &transport{
				limiter: rate.NewLimiter(rate.Every(2*time.Second), 1),
			},
		},
	}
}

func (ks *KingShot) playerStore() PlayerStore {
	if ks.store != nil {
		return ks.store
	}
	return csvstore.New(ks.playerIDFile)
}

// --- Discord handler wiring ---------------------------------------------------

// Register adds the KingShot interaction and message handlers to s once at startup.
func (ks *KingShot) Register(s *discordgo.Session, giftCodeChannelID string) {
	s.AddHandler(ks.InteractionHandler())
	s.AddHandler(ks.MessageHandler(giftCodeChannelID))
}

// GiftCodeCommands returns the slash command definitions for the KingShot gift
// code system. Register these once in the Ready handler for the NXG guild.
func (ks *KingShot) GiftCodeCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
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
}

// InteractionHandler returns a handler that dispatches /register and /code
// interactions. Register this once at startup via session.AddHandler.
func (ks *KingShot) InteractionHandler() func(s *discordgo.Session, i *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		switch i.ApplicationCommandData().Name {
		case "register":
			ks.registerPlayer(s, i)
		case "code":
			ks.addCode(s, i)
		}
	}
}

func (ks *KingShot) registerPlayer(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		slog.Error("failed to defer interaction response for register", "error", err)
		return
	}

	playerID := i.ApplicationCommandData().Options[0].StringValue()
	discordID := i.Member.User.ID

	loginResp, err := ks.login(playerID)
	if err != nil {
		slog.Error("failed to call login endpoint", "error", err)
		reply(s, i, "Error validating player ID. Please try again later.")
		return
	}
	if loginResp.Code != 0 {
		slog.Info("invalid player id", "player_id", playerID, "response_code", loginResp.Code)
		reply(s, i, "Invalid player ID provided.")
		return
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	existingDiscordID, found, err := ks.playerStore().GetDiscordID(playerID)
	if err != nil {
		slog.Error("failed to read player file for registration", "error", err)
		reply(s, i, "Error registering player ID.")
		return
	}

	if found {
		if existingDiscordID == discordID {
			reply(s, i, "This player ID is already registered to your Discord account.")
		} else {
			reply(s, i, "This player ID is already registered to another Discord account.")
		}
		return
	}

	if err := ks.playerStore().Upsert(playerID, discordID); err != nil {
		slog.Error("failed to write player file", "error", err)
		reply(s, i, "Error registering player ID.")
		return
	}

	slog.Info("user subscribed to bot", "player_id", playerID, "discord_id", discordID)

	response := "**Registration Successful!**\n**Your player ID *" + playerID + "* has been registered successfully!**"
	response += ks.redeemActiveCodes(playerID)

	reply(s, i, response)
}

func (ks *KingShot) addCode(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		slog.Error("failed to defer interaction response for code", "error", err)
		return
	}

	if !hasCodePermission(i.Member) {
		reply(s, i, "You do not have permission to use this command.")
		return
	}

	newCode := i.ApplicationCommandData().Options[0].StringValue()
	reply(s, i, fmt.Sprintf("Code %s received: processing...", newCode))

	result := ks.processNewCode(newCode)
	for _, chunk := range chunkMessage(result, discordMaxMessageLen) {
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: chunk})
	}
}

// MessageHandler returns a handler that watches channelID for bot-posted gift
// codes and triggers automatic redemption. Register once at startup.
func (ks *KingShot) MessageHandler(channelID string) func(s *discordgo.Session, m *discordgo.MessageCreate) {
	codeRegex := regexp.MustCompile(`Gift Code: ([A-Z0-9]+)`)

	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
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

		thinkingMsg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Processing new gift code: `%s`...", newCode))
		if err != nil {
			slog.Error("failed to send thinking message", "error", err)
		}

		result := ks.processNewCode(newCode)

		if thinkingMsg != nil {
			_, err := s.ChannelMessageEdit(thinkingMsg.ChannelID, thinkingMsg.ID, result)
			if err != nil {
				slog.Error("failed to edit message with result", "error", err)
				s.ChannelMessageSend(m.ChannelID, result)
			}
		} else {
			s.ChannelMessageSend(m.ChannelID, result)
		}
	}
}

// --- Core gift code business logic -------------------------------------------

// redeemOutcome captures the structured result of a single redemption attempt.
type redeemOutcome struct {
	msg         string // player-facing result message
	codeExpired bool   // true when ErrCodeExpired — remove from active list
	codeInvalid bool   // true when ErrCodeNotFound — do not add to active list
	loginFailed bool   // true when ErrCodeLogin — do not add to active list
}

// shouldAddToActive reports whether the code can be added to the active list.
func (o redeemOutcome) shouldAddToActive() bool {
	return !o.codeExpired && !o.codeInvalid && !o.loginFailed
}

// interpretRedeemResult maps a KingShot API error code to a structured outcome.
// This is a pure function — no I/O, no state.
func interpretRedeemResult(errCode ErrCode) redeemOutcome {
	switch string(errCode) {
	case ErrCodeSuccess:
		return redeemOutcome{msg: "Successfully redeemed!"}
	case ErrCodeClaimed:
		return redeemOutcome{msg: "Already claimed."}
	case ErrCodeExpired:
		return redeemOutcome{msg: "Code expired or not found.", codeExpired: true}
	case ErrCodeNotFound:
		return redeemOutcome{msg: "Code is not valid.", codeInvalid: true}
	case ErrCodeLogin:
		return redeemOutcome{msg: "Unable to login.", loginFailed: true}
	case ErrCodeLimitReached:
		return redeemOutcome{msg: "Redemption Limit Reached"}
	default:
		return redeemOutcome{msg: "Failed to redeem code."}
	}
}

// isCodeKnown reports whether code is already tracked by the gift code system.
// Caller must hold ks.mu.
func (ks *KingShot) isCodeKnown(code string) (active, expired bool) {
	return slices.Contains(ks.activeCodes, code), slices.Contains(ks.expiredCodes, code)
}

// loadPlayerIDs reads the player CSV file and returns all player IDs in row order.
func (ks *KingShot) loadPlayerIDs() ([]string, error) {
	return ks.playerStore().ListPlayerIDs()
}

// redeemForPlayer logs playerID in, redeems code, and returns a human-readable
// result message.
func (ks *KingShot) redeemForPlayer(playerID, code string) string {
	if _, err := ks.login(playerID); err != nil {
		slog.Error("failed to login", "error", err, "player_id", playerID)
		return "Failed to login."
	}
	resp, err := ks.redeemGiftCode(playerID, code)
	if err != nil {
		slog.Error("failed to redeem", "error", err, "player_id", playerID, "code", code)
		return "Error redeeming code."
	}
	slog.Info("redeem response", "player_id", playerID, "code", code, "err_code", resp.ErrCode)
	return interpretRedeemResult(resp.ErrCode).msg
}

// formatRedemptionReport builds the final summary message after a code has been
// added and redeemed for all players. Pure function — no I/O, no state.
func formatRedemptionReport(code string, playerCount int, results []string) string {
	return fmt.Sprintf(
		"Code `%s` has been added to the active list.\n\n**Redemption Results for %d players:**\n%s",
		code, playerCount, strings.Join(results, "\n"),
	)
}

// processNewCode validates newCode against the KingShot API and redeems it for
// all registered players. It is safe to call concurrently.
func (ks *KingShot) processNewCode(newCode string) string {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if active, expired := ks.isCodeKnown(newCode); active {
		return fmt.Sprintf("Code `%s` is already active.", newCode)
	} else if expired {
		return fmt.Sprintf("Code `%s` has expired and cannot be re-added.", newCode)
	}

	playerIDs, err := ks.loadPlayerIDs()
	if err != nil {
		return fmt.Sprintf("Code `%s` has not been added, as we failed to open player file.", newCode)
	}

	if len(playerIDs) == 0 {
		ks.activeCodes = append(ks.activeCodes, newCode)
		slog.Info("code added with no registered players", "code", newCode)
		return fmt.Sprintf("There are no registered players, but code `%s` has been added to the active list.", newCode)
	}

	firstPlayerID := playerIDs[0]
	if _, err := ks.login(firstPlayerID); err != nil {
		slog.Error("failed to login before validating new code", "error", err)
		return fmt.Sprintf("Failed to validate code `%s` due to an error. The code has not been added.", newCode)
	}

	redeemResp, err := ks.redeemGiftCode(firstPlayerID, newCode)
	if err != nil {
		slog.Error("failed to validate new code", "error", err, "code", newCode)
		return fmt.Sprintf("Failed to validate code `%s` due to an error. The code has not been added.", newCode)
	}

	slog.Info("redeem response", "code", newCode, "err_code", redeemResp.ErrCode, "player_id", firstPlayerID)

	outcome := interpretRedeemResult(redeemResp.ErrCode)
	if outcome.codeExpired {
		ks.expiredCodes = append(ks.expiredCodes, newCode)
		return fmt.Sprintf("Code `%s` has expired and was not added.", newCode)
	}
	if outcome.codeInvalid {
		return fmt.Sprintf("Code `%s` is not valid and was not added.", newCode)
	}
	if outcome.loginFailed {
		return fmt.Sprintf("Code `%s` could not be validated - unable to login.", newCode)
	}

	ks.activeCodes = append(ks.activeCodes, newCode)
	slog.Info("code added", "code", newCode)

	results := make([]string, 0, len(playerIDs))
	results = append(results, fmt.Sprintf("Player `%s`: %s", firstPlayerID, outcome.msg))
	for _, playerID := range playerIDs[1:] {
		results = append(results, fmt.Sprintf("Player `%s`: %s", playerID, ks.redeemForPlayer(playerID, newCode)))
	}

	return formatRedemptionReport(newCode, len(playerIDs), results)
}

// redeemActiveCodes redeems all currently active codes for playerID and returns
// a formatted summary string. Caller must hold ks.mu.
func (ks *KingShot) redeemActiveCodes(playerID string) string {
	if len(ks.activeCodes) == 0 {
		return ""
	}

	var results []string
	var codesToRemove []string

	for _, code := range ks.activeCodes {
		redeemResp, err := ks.redeemGiftCode(playerID, code)
		if err != nil {
			slog.Error("failed to redeem gift code after registration", "error", err, "code", code, "player_id", playerID)
			results = append(results, fmt.Sprintf("`%s`: Error redeeming code.", code))
			continue
		}
		slog.Info("redeem response", "code", code, "err_code", redeemResp.ErrCode, "player_id", playerID)

		outcome := interpretRedeemResult(redeemResp.ErrCode)
		if outcome.codeExpired || outcome.codeInvalid {
			codesToRemove = append(codesToRemove, code)
		}
		results = append(results, fmt.Sprintf("`%s`: %s", code, outcome.msg))
	}

	if len(codesToRemove) > 0 {
		ks.activeCodes = slices.DeleteFunc(ks.activeCodes, func(c string) bool {
			return slices.Contains(codesToRemove, c)
		})
	}

	return "\n\n**Gift Code Redemption Results:**\n" + strings.Join(results, "\n")
}

// --- KingShot API client -----------------------------------------------------

// login authenticates fid with the KingShot API.
func (ks *KingShot) login(fid string) (*LoginResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"time": fmt.Sprintf("%d", time.Now().Unix()),
	}
	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}
	resp, err := ks.client.Post(ks.loginURL, "application/json", strings.NewReader(payload))
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

// redeemGiftCode submits a redemption request for fid and cdk.
func (ks *KingShot) redeemGiftCode(fid, cdk string) (*RedeemResponse, error) {
	data := map[string]string{
		"fid":  fid,
		"cdk":  cdk,
		"time": fmt.Sprintf("%d", time.Now().Unix()),
	}
	payload, err := EncodePayload(data)
	if err != nil {
		return nil, err
	}
	resp, err := ks.client.Post(ks.redeemURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var redeemResp RedeemResponse
	if err := json.NewDecoder(resp.Body).Decode(&redeemResp); err != nil {
		return nil, fmt.Errorf("failed to decode redemption response: %w", err)
	}
	return &redeemResp, nil
}

// --- Payload encoding --------------------------------------------------------

// EncodePayload encodes the data map into a signed JSON payload for the
// KingShot API. It adds a "sign" field to data as a side effect.
func EncodePayload(data map[string]string) (string, error) {
	values := url.Values{}
	for key, value := range data {
		values.Set(key, value)
	}

	hasher := md5.New()
	hasher.Write([]byte(values.Encode() + Key))
	data["sign"] = hex.EncodeToString(hasher.Sum(nil))

	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// --- API response types -------------------------------------------------------

// ErrCode handles the KingShot API's habit of returning err_code as either a
// JSON string or a JSON number.
type ErrCode string

func (e *ErrCode) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*e = ErrCode(s)
		return nil
	}
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*e = ErrCode(strconv.Itoa(i))
		return nil
	}
	return fmt.Errorf("err_code is not a string or a number: %s", data)
}

type LoginResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	Data    any     `json:"data"`
	ErrCode ErrCode `json:"err_code"`
}

type RedeemResponse struct {
	Code    int     `json:"code"`
	Message string  `json:"msg"`
	ErrCode ErrCode `json:"err_code"`
}

// --- Helpers -----------------------------------------------------------------

// hasCodePermission returns true if the member is an administrator or has the
// r4 role.
func hasCodePermission(member *discordgo.Member) bool {
	if member.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
		return true
	}
	for _, roleID := range member.Roles {
		if roleID == r4RoleId {
			return true
		}
	}
	return false
}

// chunkMessage splits s into slices of at most maxLen characters, breaking on
// newline boundaries where possible.
func chunkMessage(s string, maxLen int) []string {
	if len(s) <= maxLen {
		return []string{s}
	}
	var chunks []string
	for len(s) > 0 {
		end := maxLen
		if len(s) < end {
			end = len(s)
		}
		if idx := strings.LastIndex(s[:end], "\n"); idx != -1 {
			end = idx
		}
		chunks = append(chunks, s[:end])
		s = strings.TrimPrefix(s[end:], "\n")
	}
	return chunks
}

// reply edits the deferred interaction response with the given message.
func reply(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	if err != nil {
		slog.Error("failed to edit interaction response", "error", err)
	}
}
