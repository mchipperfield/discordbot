package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mchipperfield/discordbot/dca"
	"github.com/peterbourgon/ff"

	"github.com/bwmarrin/discordgo"
	"github.com/mchipperfield/discordbot/ai"
)

var quotes = []string{
	"honestly",
	"sumptuous",
	"in my tenure",
	"the thiinnng is",
	"#cheers",
	"You know where it is",
	"it's not too bad actually",
	"i love the chocolate starfish",
	"i'm always prepared!",
	"Up the Spurs!",
	"SEND OUT!!!",
	"You are ENOUGH!",
	"I, do NOT fail!",
}

var commands = []*discordgo.ApplicationCommand{
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

// Server2985 is the guild id for the SD server.
const (
	Server2985 = "1339671620880699433"
	ServerNXG  = "1423406563850190850"
)

func main() {
	logger := logger{slog.Default()}

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	var (
		token        = fs.String("bot_token", "", "bot authentication token")
		serverId     = fs.String("server_id", Server2985, "server to listen on")
		nxg          = fs.String("nxg_server_id", ServerNXG, "NXG server id")
		spellingURL  = fs.String("spelling_url", "https://gist.githubusercontent.com/ZekNikZ/5e7dd531df99be4408bd768ded36aad9/raw/c0ecc900022d60d54accb3770f2e737dcba738ad/british-american-words.txt", "URL to uk-us dictionary file")
		geminiAPIKey = fs.String("gemini_api_key", "", "API key for Gemini AI service")
		playerIDFile = fs.String("player_id_file", "player_ids.csv", "File to store player IDs")
	)
	if err := ff.Parse(fs,
		os.Args[1:],
		ff.WithEnvVarNoPrefix(),
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
		logger.Info("failed to create ai service", "error", err)
		os.Exit(1)
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

	session.AddHandler(AskGemini(aiService))
	session.AddHandler(RegisterPlayer(*playerIDFile))
	session.AddHandler(AddCode(*playerIDFile))
	session.AddHandler(hungry(*serverId))
	session.AddHandler(getQuote(*serverId, quotes))
	session.AddHandler(wakeUp(*serverId, dcaService.GetSound("wake_up.dca")))
	session.AddHandler(Kit(*nxg))
	session.AddHandler(listen(*nxg, dcaService.GetSound("hey_listen.dca")))
	session.AddHandler(americanSpellingPolice(spellings))
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("Bot is up!", "user", r.User.String(), "session_id", r.SessionID, "version", r.Version)

		// Clean up old commands to ensure a fresh state.
		existingCommands, err := s.ApplicationCommands(s.State.User.ID, *nxg)
		if err != nil {
			logger.Info("could not fetch existing commands", "error", err)
		} else {
			for _, v := range existingCommands {
				err := s.ApplicationCommandDelete(s.State.User.ID, "", v.ID)
				if err != nil {
					logger.Info("cannot delete command", "command", v.Name, "error", err)
				}
			}
		}

		// Register new commands.
		for _, v := range commands {
			_, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
			if err != nil {
				logger.Info("cannot create command", "command", v.Name, "error", err)
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
