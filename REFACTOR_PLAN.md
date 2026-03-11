# Refactor Plan

## Context

This bot serves two Discord servers with completely separate purposes:

| Constant | Guild ID | Purpose |
|---|---|---|
| `Server2985` ("SD") | `1339671620880699433` | Social server — fun reactions, quotes, voice gags |
| `ServerNXG` ("NXG") | `1423406563850190850` | Gaming server — KingShot gift codes, AI Q&A, cat pics |

The current codebase mixes both servers' handlers into the same files, has a race condition on shared global state, and has at least four distinct duplication patterns. Every step below follows the same discipline: **write tests first, then change the code.**

---

## Principles

- **One thing at a time** — each step is a single, reviewable PR
- **Tests before refactor** — tests capture current behaviour; refactoring must keep them green
- **No behaviour changes** — each step is purely structural unless a bug is being fixed as part of the step (noted explicitly)
- **SOLID** — Single Responsibility, Open/Closed, Liskov, Interface Segregation, Dependency Inversion applied where they reduce complexity, not as dogma

---

## Step 1 — Fix the race condition: introduce a `KingShot` struct

### Problem
`ActiveCodes`, `ExpiredCodes`, and `playerIDMutex` are package-level globals in `kingshot.go`.
`ActiveCodes` and `ExpiredCodes` are read and written from both `registerPlayer` and `processNewCode` without holding the mutex, creating a real data race.

### What changes
- Introduce a `KingShot` struct that owns `activeCodes []string`, `expiredCodes []string`, `mu sync.Mutex`, `playerIDFile string`, and `httpClient *http.Client`
- Move `Login`, `RedeemGiftCode`, `processNewCode`, `registerPlayer`, `addCode`, `messageHandler` to be methods on `*KingShot`
- All reads/writes to `activeCodes` and `expiredCodes` go inside `mu.Lock()` / `mu.Unlock()`
- Delete the three package-level vars

### Tests to write first
- `TestKingShotNewCode_AlreadyActive` — duplicate of `TestProcessNewCodeAlreadyActive` but using the struct
- `TestKingShotNewCode_AlreadyExpired`
- `TestKingShotNewCode_FileNotFound`
- `TestKingShotNewCode_EmptyPlayerFile` — code added to active list, empty file returns the right message
- Run `go test -race ./...` — this is the acceptance criterion; the race detector must be silent

### Files touched
`kingshot.go`, `kingshot_test.go`

---

## Step 2 — Extract pure business logic from `processNewCode`

### Problem
`processNewCode` is ~120 lines and does five distinct things:
1. Duplicate-code check
2. Read the player CSV
3. Validate the code against the first player via the API
4. Redeem the code for all remaining players
5. Format the result string

None of steps 1–4 can be tested independently because the logic is inlined.

### What changes
Extract the following methods on `*KingShot`:
```
isCodeKnown(code string) (active bool, expired bool)
loadPlayers(file string) ([]string, error)
validateCode(playerID, code string) (resultMsg string, shouldAdd bool, err error)
redeemForPlayer(playerID, code string) (resultMsg string, err error)
formatRedemptionReport(code string, results []string) string
```
`processNewCode` becomes a thin orchestrator that calls these in order.

### Tests to write first
- `TestIsCodeKnown` — table-driven: active code, expired code, unknown code
- `TestLoadPlayers` — valid CSV, empty file, malformed rows, missing file
- `TestValidateCode` — use `httptest.NewServer` to mock the KingShot API:
  - success response → `shouldAdd = true`
  - expired response → `shouldAdd = false`, code added to `expiredCodes`
  - not-found response → `shouldAdd = false`
  - login failure response
- `TestRedeemForPlayer` — same mock server, table-driven on each ErrCode
- `TestFormatRedemptionReport` — pure string formatting, no I/O

### Files touched
`kingshot.go`, `kingshot_test.go`

---

## Step 3 — Extract trigger predicates from all message handlers

### Problem
Every handler embeds its trigger condition inline (`strings.Contains`, `regexp.MatchString`) mixed with Discord session calls. This means the detection logic cannot be tested without a real Discord session.

### What changes
Extract one pure function per handler that answers "does this message content match?":

```go
// handlers.go (or new file handler_predicates.go)
func isHungry(content string) bool
func isTired(content string) bool
func isKit(content string) bool
func isFullSend(content string) bool
func isListen(content string) bool
```

Move all compiled regexes to package-level `var` (they are already closures — make them explicit).
Each handler calls the predicate and does nothing else if it returns false.

### Tests to write first (new file `handler_predicates_test.go`)
Table-driven tests for each predicate:

| Predicate | True cases | False cases |
|---|---|---|
| `isHungry` | `"I'm hungry"`, `"HUNGRY"` | `"I'm full"`, `""` |
| `isTired` | `"so tired"`, `"TIRED today"` | `"not sleepy"`, `""` |
| `isKit` | `"got a kit"`, `"KIT"` | `"kitten"`, `"skit"` (word boundary) |
| `isFullSend` | `"full send it"`, `"FULL SEND"` | `"fullsend"`, `"half send"` |
| `isListen` | `"listen up"`, `"LISTEN"` | `"listening"` (word boundary) |

### Files touched
`handlers.go` (or new `handler_predicates.go`), new `handler_predicates_test.go`

---

## Step 4 — Eliminate voice channel duplication

### Problem
`wakeUp` and `listen` contain identical 24-line blocks for joining a voice channel, playing opus frames, and disconnecting.
There is also a latent panic: if `s.State.Guild()` returns an error, the code logs it but proceeds to iterate `guild.VoiceStates` on the nil pointer.

### What changes
Extract a standalone function:
```go
// playAudioToUser joins the voice channel of userID in guildID and plays opusFrames.
// Returns an error if the user is not in any voice channel.
func playAudioToUser(s *discordgo.Session, guildID, userID string, opusFrames [][]byte) error
```

Fix the nil-guild panic: return the error from `s.State.Guild()` instead of continuing.
Add a `break` after sending audio (no reason to check other voice states after the user is found).

`wakeUp` and `listen` become:
```go
if err := playAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
    slog.Info("could not play audio", "error", err, "user", m.Author.ID)
}
```

### Tests to write first
`playAudioToUser` needs a Discord session — we cannot easily unit-test the happy path without a live connection. Instead:
- Test the **guard rail**: write a test that confirms `playAudioToUser` returns a descriptive error (not a panic) when the guild is not found
- Use a `discordgo.Session` with an empty state — no real network needed for this path
- The rest (actual audio playback) is integration-level; mark it with `t.Skip("requires live Discord")` if desired

### Files touched
`handlers.go` (or new `voice.go`), new `voice_test.go`

---

## Step 5 — Extract the guard-clause middleware

### Problem
This exact six-line block appears in all six message handlers:
```go
if m.GuildID != serverId {
    return
}
if m.Author.ID == s.State.User.ID {
    return
}
```
36 lines of copy-paste that must all be updated if the logic ever changes.

### What changes
Extract:
```go
// onMessage wraps a handler so it only fires for the given guild and never for the bot's own messages.
func onMessage(guildID string, h func(*discordgo.Session, *discordgo.MessageCreate)) func(*discordgo.Session, *discordgo.MessageCreate) {
    return func(s *discordgo.Session, m *discordgo.MessageCreate) {
        if m.GuildID != guildID {
            return
        }
        if m.Author.ID == s.State.User.ID {
            return
        }
        h(s, m)
    }
}
```

Registration in `main.go` becomes:
```go
session.AddHandler(onMessage(*serverId, handleHungry))
session.AddHandler(onMessage(*serverId, handleGetQuote(quotes)))
session.AddHandler(onMessage(*serverId, handleWakeUp(dcaService.GetSound("wake_up.dca"))))
session.AddHandler(onMessage(*nxg, handleKit))
session.AddHandler(onMessage(*nxg, handleListen(dcaService.GetSound("hey_listen.dca"))))
session.AddHandler(onMessage(*nxg, handleBlondie))
```

### Tests to write first (new file `middleware_test.go`)
- `TestOnMessage_WrongGuild` — handler must not be called
- `TestOnMessage_BotSelf` — handler must not be called
- `TestOnMessage_Fires` — handler called with correct guild and non-bot author

### Files touched
New `middleware.go`, new `middleware_test.go`, `handlers.go`, `main.go`

---

## Step 6 — Fix the `GiftCodeCommandHandler` Ready-handler anti-pattern

### Problem
`GiftCodeCommandHandler` returns a `Ready` handler that, every time the bot connects, both registers slash commands **and** calls `s.AddHandler(...)` inside the running session. On reconnect, duplicate handlers accumulate and fire multiple times per event.

Additionally, main.go's own Ready handler deletes global commands (`""` scope) but `GiftCodeCommandHandler` creates guild-scoped commands — so the cleanup never removes them.

### What changes
1. Move slash command registration out of the Ready handler into a dedicated `registerCommands(s *discordgo.Session, guildID string, commands []*discordgo.ApplicationCommand) error` function called once after `session.Open()`
2. Move `s.AddHandler(messageHandler(...))` and the interaction handler to `main.go` alongside all other `session.AddHandler(...)` calls
3. Delete `GiftCodeCommandHandler` entirely
4. Unify the command cleanup in main.go's Ready handler to cover the NXG guild as well

### Tests to write first
- `TestRegisterCommands_CreatesExpectedCommands` — mock the Discord API with `httptest.NewServer`, verify the correct slash commands are created with the right names and options
- This is more of an integration smoke test; the main value is documenting the expected command set

### Files touched
`kingshot.go`, `main.go`

---

## Step 7 — Split into per-server packages

### Problem
All handler logic lives in `main` package files. There is no visible boundary between SD and NXG. Adding a new SD feature requires opening the same file as NXG features.

### Proposed package layout
```
discordbot/
  main.go                    ← wiring only: flags, session, AddHandler calls
  middleware.go              ← onMessage guard wrapper
  spelling.go                ← spelling utilities (already clean)
  ai/
    handler.go               ← existing, unchanged
  dca/
    dca.go                   ← existing, unchanged
  server/
    sd/
      handlers.go            ← hungry, getQuote, wakeUp
      handlers_test.go
    nxg/
      handlers.go            ← Kit, Blondie, listen, AskGemini
      handlers_test.go
  kingshot/
    service.go               ← KingShot struct, Login, RedeemGiftCode
    service_test.go
    store.go                 ← loadPlayers, CSV read/write
    store_test.go
    handler.go               ← registerPlayer, addCode, messageHandler (Discord layer)
    handler_test.go
```

Each sub-package exposes a `Register(s *discordgo.Session, cfg Config)` function. `main.go` calls each one in turn — it knows nothing about how handlers are implemented.

### Tests to write first
- Each package's `handlers_test.go` tests the predicate functions and any pure business logic within that package
- Integration tests for `Register(...)` can be marked `t.Skip` until a proper mock session is available

### Files touched
All files — this is the final structural step. Done last because all previous steps must be green.

---

## Issue Backlog (not blocking, address opportunistically)

| Issue | Location | Fix |
|---|---|---|
| `r4RoleId` hardcoded | `kingshot.go:39` | Make it a flag in `main.go` passed into `KingShot` |
| KingShot API `Key` exposed in source | `kingshot.go:28` | Move to flag / environment variable |
| Gemini model name hardcoded | `ai/handler.go:34` | Const or config field |
| `1900` magic number | `kingshot.go:296,300` | `const discordMaxMessageLen = 2000` / use the right value |
| Response chunking logic | `addCode()` | Extract `chunkMessage(s string, maxLen int) []string` |
| `loadSound` prints to stdout | `dca/dca.go:47,63,67` | Replace with returned error / logger |
| `EncodePayload` mutates its argument | `kingshot.go:517` | Return a new map or separate sign from payload |

---

## Acceptance Criteria (applies to every step)

1. `go test -race ./...` passes with no failures and no race warnings
2. `go vet ./...` passes cleanly
3. No change in observable bot behaviour
4. Each step is committed independently so it can be reviewed and reverted in isolation
