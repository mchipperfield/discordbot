package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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
}

// Server2985 is the guild id for the SD server.
const Server2985 = "1339671620880699433"

func main() {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	var (
		token    = fs.String("bot_token", "", "bot authentication token")
		serverId = fs.String("server_id", Server2985, "server to listen on")
	)
	if err := ff.Parse(fs,
		os.Args[1:],
		ff.WithEnvVarNoPrefix(),
	); err != nil {
		slog.Info("failed to parse flags", "error", err)
		os.Exit(1)
	}

	opus, err := loadSound("wake_up.dca")
	if err != nil {
		slog.Info("failed to load sound", "error", err)
		os.Exit(1)
	}

	session, err := discordgo.New("Bot " + *token)
	if err != nil {
		slog.Info("failed to create session", "error", err)
		os.Exit(1)
	}

	session.AddHandler(hungry(*serverId))
	session.AddHandler(getQuote(*serverId, quotes))
	session.AddHandler(wakeUp(*serverId, opus))

	if err := session.Open(); err != nil {
		slog.Info("error opening websocket", "error", err)
		os.Exit(1)
	}
	slog.Info("websocket established")

	defer func() {
		slog.Info("closing websocket")
		if err := session.Close(); err != nil {
			slog.Info("error closing websocket", "error", err)
		}
	}()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stopChan

	slog.Info("signal received", "signal", sig)

}

// loadSound copy/pasta from the !airhorn example at bwmarrin/godiscord
func loadSound(filename string) ([][]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening dca file :", err)
		return nil, err
	}
	var buffer = make([][]byte, 0)
	var opuslen int16

	for {
		// Read opus frame length from dca file.
		err = binary.Read(file, binary.LittleEndian, &opuslen)

		// If this is the end of the file, just return.
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err := file.Close()
			if err != nil {
				return nil, err
			}
			return buffer, nil
		}

		if err != nil {
			fmt.Println("Error reading from dca file :", err)
			return nil, err
		}

		// Read encoded pcm from dca file.
		InBuf := make([]byte, opuslen)
		err = binary.Read(file, binary.LittleEndian, &InBuf)

		// Should not be any end of file errors
		if err != nil {
			fmt.Println("Error reading from dca file :", err)
			return nil, err
		}

		// Append encoded pcm data to the buffer.
		buffer = append(buffer, InBuf)
	}

}
