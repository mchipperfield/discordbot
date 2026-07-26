package main

import (
	"bufio"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
	"github.com/mchipperfield/discordbot/dca"
	"github.com/mchipperfield/discordbot/kingshot"
	ksdiscord "github.com/mchipperfield/discordbot/kingshot/discord"
	kingshotcsv "github.com/mchipperfield/discordbot/kingshot/storage/csv"
	"github.com/mchipperfield/discordbot/middleware"
	"github.com/mchipperfield/discordbot/server/nxg"
	"github.com/mchipperfield/discordbot/server/sd"
	"github.com/peterbourgon/ff"
)

var askCommand = []*discordgo.ApplicationCommand{
	{
		Name:        "ask",
		Description: "Ask the bot a question",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "The question you want to ask",
				Required:    true,
			},
		},
	},
}

// Server2985 is the guild ID for the SD server.
// ServerNXG is the guild ID for the NXG gaming server.
const (
	Server2985 = "1339671620880699433"
	ServerNXG  = "1423406563850190850"
	ServerWHS  = "1479709703155093587"
	ServerSTR  = "1418924707373383774"
	ServerBSV  = "1459254024703316009"
)

func main() {
	logger := logger{slog.Default()}

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	var (
		token             = fs.String("bot_token", "", "bot authentication token")
		serverId          = fs.String("server_id", Server2985, "server to listen on")
		nxgID             = fs.String("nxg_server_id", ServerNXG, "NXG server id")
		spellingURL       = fs.String("spelling_url", "https://gist.githubusercontent.com/ZekNikZ/5e7dd531df99be4408bd768ded36aad9/raw/c0ecc900022d60d54accb3770f2e737dcba738ad/british-american-words.txt", "URL to uk-us dictionary file")
		geminiAPIKey      = fs.String("gemini_api_key", "", "API key for Gemini AI service")
		playerIDFile      = fs.String("player_id_file", "player_ids.csv", "File to store player IDs")
		giftCodeChannelID = fs.String("gift_code_channel_id", "1428776775621673001", "Channel ID to listen for gift codes in")
		goafChannelID     = fs.String("goaf_channel_id", "", "Channel ID to post bear alerts in")
		goafStateFile     = fs.String("goaf_state_file", "goaf_state.json", "File to persist GOAF alert state")
	)
	if err := ff.Parse(fs,
		os.Args[1:],
		ff.WithEnvVarNoPrefix(),
		ff.WithConfigFile(".env"),
		ff.WithConfigFileParser(dotEnvParser),
	); err != nil {
		logger.Log("failed to parse flags", "error", err)
		os.Exit(1)
	}

	spellings, err := LoadSpellingsFromURL(*spellingURL)
	if err != nil {
		logger.Info("failed to load spellings", "error", err)
		os.Exit(1)
	}

	aiService, err := ai.NewService(*geminiAPIKey)
	if err != nil {
		logger.Info("no AI service available, /ask command will be disabled", "reason", err)
	}

	dcaService, err := dca.NewService(logger)
	if err != nil {
		logger.Info("failed to create dca service", "error", err)
		os.Exit(1)
	}

	session, err := discordgo.New("Bot " + *token)
	if err != nil {
		logger.Info("failed to create discord session", "error", err)
		os.Exit(1)
	}

	ks := kingshot.NewKingShotWithStore(
		kingshotcsv.New(*playerIDFile),
		"PICNIC2026", "AJISAI26JP", "Kingshot888", "VIP777",
	)
	goafSvc := nxg.NewGoafService(*goafChannelID, *goafStateFile)

	// Register all handlers once at startup — never inside a Ready callback.
	sd.Register(session, *serverId, dcaService.GetSound("wake_up.dca"))
	nxg.Register(session, *nxgID, dcaService.GetSound("hey_listen.dca"), aiService)
	goafSvc.Register(session)
	ksdiscord.Register(session, ks, *giftCodeChannelID)
	session.AddHandler(middleware.OnAnyMessage(americanSpellingPolice(spellings)))
	session.AddHandler(middleware.OnMessage(*nxgID, shoutingPolice(shoutingTargetUserID)))
	session.AddHandler(middleware.OnMessage(ServerBSV, shoutingPolice(shoutingTargetUserID)))

	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("Bot is up!", "user", r.User.String(), "session_id", r.SessionID, "version", r.Version)

		// Clean up old commands to ensure a fresh state.
		for _, guildID := range []string{"", *nxgID, ServerWHS, ServerSTR, ServerBSV} {
			existing, err := s.ApplicationCommands(s.State.User.ID, guildID)
			if err != nil {
				logger.Info("could not fetch existing commands", "guild", guildID, "error", err)
				continue
			}
			for _, v := range existing {
				if err := s.ApplicationCommandDelete(s.State.User.ID, guildID, v.ID); err != nil {
					logger.Info("cannot delete command", "command", v.Name, "error", err)
				}
			}
		}

		// Register global commands.
		for _, v := range askCommand {
			if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", v); err != nil {
				logger.Info("cannot create command", "command", v.Name, "error", err)
			}
		}

		// Register NXG guild commands.
		nxgCommands := append(ksdiscord.GiftCodeCommands(ks), goafSvc.GoafCommands()...)
		for _, v := range nxgCommands {
			if _, err := s.ApplicationCommandCreate(s.State.User.ID, *nxgID, v); err != nil {
				logger.Error("cannot create command", "command", v.Name, "error", err)
			}
		}

		for _, guildID := range []string{ServerWHS, ServerSTR, ServerBSV} {
			for _, v := range ksdiscord.GiftCodeCommands(ks) {
				if _, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, v); err != nil {
					logger.Error("cannot create command", "command", v.Name, "error", err)
				}
			}
		}

	})

	if err := session.Open(); err != nil {
		logger.Info("error opening websocket", "error", err)
		os.Exit(1)
	}
	logger.Info("websocket established")

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stopChan

	logger.Info("signal received", "signal", sig)
}

type logger struct {
	*slog.Logger
}

func (l logger) Log(msg string, keyvals ...any) error {
	l.Logger.Info(msg, keyvals...)
	return nil
}

// dotEnvParser reads a .env file in KEY=VALUE format (one per line).
// Lines starting with # and blank lines are ignored.
func dotEnvParser(r io.Reader, set func(name, value string) error) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if err := set(name, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
