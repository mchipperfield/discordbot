# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run all tests (always use -race)
go test -race ./...

# Run a single test
go test -race -run TestKingShot_processNewCode ./kingshot/

# Run tests in a specific package
go test -race ./server/sd/

# Build
go build ./...

# Vet
go vet ./...

# Run the bot (reads .env in working directory)
go run .
```

## Architecture

This is a Go Discord bot serving two separate Discord servers:

| Constant | Guild ID | Server |
|---|---|---|
| `Server2985` | `1339671620880699433` | SD — social server (quotes, hungry gag, wake-up audio) |
| `ServerNXG` | `1423406563850190850` | NXG — gaming server (KingShot gift codes, cat pics, AI Q&A) |

### Package layout

```
main.go              — flag parsing, dependency wiring, calls Register()
spelling.go          — cross-server American→British spelling police (fires on all guilds)
middleware/          — OnMessage(guildID, h) and OnAnyMessage(h) guard wrappers
voice/               — PlayAudioToUser shared audio helper
server/sd/           — SD server: Register(), handlers, predicates
server/nxg/          — NXG server: Register(), handlers, predicates
kingshot/            — KingShot gift code service (HTTP client, CSV player store, slash commands)
ai/                  — Gemini AI service wrapper (optional — bot runs without it)
dca/                 — Opus audio file loader
```

### How handlers are wired

`main.go` builds all dependencies, then makes three calls:

```go
sd.Register(session, *serverId, dcaService.GetSound("wake_up.dca"))
nxg.Register(session, *nxgID, dcaService.GetSound("hey_listen.dca"), aiService)
ks.Register(session, *giftCodeChannelID)
```

Each `Register()` calls `middleware.OnMessage(guildID, handler)` internally — handlers never contain guild or bot-self guards themselves. The spelling handler uses `middleware.OnAnyMessage` (all guilds).

Slash commands (`/ask`, `/register`, `/code`) are registered in the single `Ready` callback in `main.go`. **Never call `s.AddHandler` inside a `Ready` callback** — it duplicates handlers on reconnect.

### KingShot package

`KingShot` struct owns all mutable state (`activeCodes`, `expiredCodes`, `mu`, HTTP client). Key methods:
- `processNewCode` — validates + redeems a code for all registered players
- `InteractionHandler()` / `MessageHandler(channelID)` — registered once at startup via `ks.Register()`
- `GiftCodeCommands()` — returns slash command definitions, called from the Ready handler in `main.go`

### Local development

Create a `.env` file in the project root:

```
bot_token=YOUR_DISCORD_BOT_TOKEN
server_id=YOUR_TEST_SERVER_ID
nxg_server_id=YOUR_TEST_SERVER_ID
gift_code_channel_id=SOME_CHANNEL_ID
```

`gemini_api_key` is optional — the bot logs a warning and disables `/ask` if omitted.

### Test discipline

- Every refactor step: write failing tests first, then implement
- Use `httptest.NewServer` for KingShot API mocks (see `kingshot/kingshot_test.go`)
- Use `discordgo.NewState()` + `state.GuildAdd()` to seed Discord state without a live connection
- `go test -race ./...` must pass clean before every commit
