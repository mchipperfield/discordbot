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

// Server2985 is the guild id for the SD server.
const (
	Server2985 = "1339671620880699433"
	ServerNXG  = "1423406563850190850"
)

func main() {
	logger := logger{slog.Default()}

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	var (
		token    = fs.String("bot_token", "", "bot authentication token")
		serverId = fs.String("server_id", Server2985, "server to listen on")
		nxg      = fs.String("nxg_server_id", ServerNXG, "NXG server id")
	)
	if err := ff.Parse(fs,
		os.Args[1:],
		ff.WithEnvVarNoPrefix(),
	); err != nil {
		logger.Log("failed to parse flags", "error", err)
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

	session.AddHandler(hungry(*serverId))
	session.AddHandler(getQuote(*serverId, quotes))
	session.AddHandler(wakeUp(*serverId, dcaService.GetSound("wake_up.dca")))
	session.AddHandler(Kit(*nxg))
	session.AddHandler(listen(*nxg, dcaService.GetSound("hey_listen.dca")))

	if err := session.Open(); err != nil {
		logger.Info("error opening websocket", "error", err)
		os.Exit(1)
	}
	logger.Info("websocket established")

	defer func() {
		logger.Info("closing websocket")
		if err := session.Close(); err != nil {
			logger.Info("error closing websocket", "error", err)
		}
	}()

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
