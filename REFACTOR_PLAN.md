# Refactor Plan

## Context

This bot serves two Discord servers with completely separate purposes:

| Constant | Guild ID | Purpose |
|---|---|---|
| `Server2985` ("SD") | `1339671620880699433` | Social server — fun reactions, quotes, voice gags |
| `ServerNXG` ("NXG") | `1423406563850190850` | Gaming server — KingShot gift codes, AI Q&A, cat pics |

Every step follows the same discipline: **write tests first, then change the code.**

---

## Principles

- **One thing at a time** — each step is a single, reviewable commit
- **Tests before refactor** — tests capture current behaviour; refactoring must keep them green
- **No behaviour changes** — each step is purely structural unless a bug is explicitly noted
- **SOLID** — applied where it reduces complexity, not as dogma

---

## Progress

| Step | Status | Commit |
|---|---|---|
| 1 — Introduce `KingShot` struct | ✅ Done | `fd07611` |
| 2 — Extract pure business logic from `processNewCode` | ✅ Done | `b066266` |
| 3 — Extract trigger predicates | ✅ Done | `1fd87ee` |
| 4 — Eliminate voice channel duplication | ✅ Done | `089b871` |
| 5 — Guard-clause middleware | ✅ Done | `06bedac` |
| 6 — Fix `GiftCodeCommandHandler` Ready anti-pattern | ✅ Done | `9dcc6ff` |
| 7 — Split into per-server packages | ✅ Done | `6c3cc4d` |

---

## ✅ Step 1 — Introduce `KingShot` struct *(done)*

**Problem:** Three package-level globals (`ActiveCodes`, `ExpiredCodes`, `playerIDMutex`) plus a global `httpClient` made tests either unreliable (shared state) or impossible (no HTTP injection).

**What was done:**
- `KingShot` struct owns `activeCodes`, `expiredCodes`, `mu`, `client`, `loginURL`, `redeemURL`
- All handler functions became methods; no state leaks to package scope
- `loginURL`/`redeemURL` as fields allow test servers to be injected
- `login` and `redeemGiftCode` became unexported methods using `ks.client`
- Extracted `hasCodePermission`, `chunkMessage`, `reply`, `readPlayerFile`, `writePlayerFile`
- Added `const discordMaxMessageLen = 1900`
- Fixed hardcoded `"1423406563850190850"` string in command registration to use `ServerNXG`

**Tests added:** `TestKingShot_processNewCode` (4 sub-tests), `TestKingShot_concurrentAccess`

**Acceptance:** `go test -race ./...` — 30 tests, race detector silent.

---

## ✅ Step 2 — Extract pure business logic from `processNewCode` *(done)*

**Problem:** `processNewCode` was 70 lines doing five things. Three identical `switch string(redeemResp.ErrCode)` blocks were duplicated across `processNewCode`, `redeemActiveCodes`, and `redeemForPlayers`.

**What was done:**
- `interpretRedeemResult(ErrCode) redeemOutcome` — pure function, single source of truth for all API error code interpretation. Replaced all three switch blocks.
- `isCodeKnown(code) (active, expired bool)` — readable guard, testable without I/O
- `loadPlayerIDs() ([]string, error)` — focused CSV reader returning just the ID column
- `redeemForPlayer(playerID, code) string` — single-player login+redeem+interpret pipeline
- `formatRedemptionReport(code, playerCount, results) string` — pure string formatting
- `processNewCode` is now ~30 lines reading as a clear sequence
- `redeemForPlayers` deleted — replaced by a plain loop using `redeemForPlayer`

**Tests added:** `TestInterpretRedeemResult` (6), `TestKingShot_isCodeKnown` (4), `TestKingShot_loadPlayerIDs` (4), `TestKingShot_redeemForPlayer` (5 incl. HTTP failure), `TestFormatRedemptionReport`, `TestChunkMessage` (4)

**Acceptance:** `go test -race ./...` — 47 tests, race detector silent.

---

## ✅ Step 3 — Extract trigger predicates from message handlers *(done)*

### Problem
Every handler buries its "should I fire?" logic inline, entangled with Discord session calls:

```go
// Kit (handlers.go)
if kitRegex.MatchString(strings.ToLower(m.Content)) {
    resp, err := http.Get("https://api.thecatapi.com/...") // can't test this without the check
```

The trigger conditions (`strings.Contains`, `regexp.MatchString`) cannot be tested without constructing a full Discord message object.

### What changes
New file `handler_predicates.go` with one pure function per handler trigger:

```go
func isHungry(content string) bool   // strings.Contains, case-insensitive
func isTired(content string) bool    // strings.Contains, case-insensitive
func isKit(content string) bool      // word-boundary regex \bkit\b
func isFullSend(content string) bool // word-boundary regex \bfull send\b
func isListen(content string) bool   // word-boundary regex \blisten\b
```

Package-level compiled regexes (currently re-compiled on each closure call in `Kit`, `Blondie`, `listen`) move to `var` declarations at the top of the file. Each handler body becomes a single predicate check at the top, then does its work.

### Tests to write first (new file `handler_predicates_test.go`)

| Predicate | True cases | False cases |
|---|---|---|
| `isHungry` | `"I'm hungry"`, `"HUNGRY!"` | `"I'm full"`, `""` |
| `isTired` | `"so tired today"`, `"TIRED"` | `"not sleepy"`, `""` |
| `isKit` | `"got a kit"`, `"KIT"` | `"kitten"`, `"skit"` (word boundary matters) |
| `isFullSend` | `"full send it"`, `"FULL SEND"` | `"fullsend"`, `"half send"` |
| `isListen` | `"listen up"`, `"LISTEN"` | `"listening"`, `"mislistened"` (word boundary matters) |

### Files touched
New `handler_predicates.go`, new `handler_predicates_test.go`, `handlers.go`

---

## ✅ Step 4 — Eliminate voice channel duplication *(done)*

### Problem
`wakeUp` and `listen` contain byte-for-byte identical 24-line blocks. There is also a latent nil-pointer panic: if `s.State.Guild()` returns an error the code logs it but still iterates `guild.VoiceStates` on a nil pointer.

```go
// This block is copy-pasted verbatim in both handlers
guild, err := s.State.Guild(m.GuildID)
if err != nil {
    slog.Info("failed to get guild", ...) // logs, but does NOT return
}
for _, vs := range guild.VoiceStates { // panics if guild == nil
```

### What changes
Extract to `voice.go`:

```go
// playAudioToUser joins the voice channel of userID in guildID, plays opusFrames,
// then disconnects. Returns an error if the guild or user's voice state cannot be found.
func playAudioToUser(s *discordgo.Session, guildID, userID string, opusFrames [][]byte) error
```

Fix the bug: return early on guild-not-found instead of continuing. Add `break` after sending audio — once the user is found there is no reason to keep iterating voice states.

Both handlers collapse to:
```go
if isTired(m.Content) {   // after Step 3
    if err := playAudioToUser(s, m.GuildID, m.Author.ID, opus); err != nil {
        slog.Info("could not play audio", "error", err)
    }
}
```

### Tests to write first (new file `voice_test.go`)
The happy path (actual audio playback) requires a live Discord connection — mark those `t.Skip("requires live Discord")`. What we *can* test without a network:

- `TestPlayAudioToUser_GuildNotFound` — pass a `discordgo.Session` with empty state, confirm a descriptive error is returned (not a panic)
- `TestPlayAudioToUser_UserNotInVoice` — guild exists in state but has no voice states for the user, confirm a descriptive error is returned

Both tests use `discordgo.State` directly to seed fixture data — no websocket needed.

### Files touched
New `voice.go`, new `voice_test.go`, `handlers.go`

---

## ✅ Step 5 — Guard-clause middleware *(done)*

### Problem
This block appears verbatim in all six message handlers (36 lines total):

```go
if m.GuildID != serverId {
    return
}
if m.Author.ID == s.State.User.ID {
    return
}
```

`americanSpellingPolice` is the odd one out — it ignores self-messages but has **no guild filter**, meaning it fires on every server the bot is in. This is likely unintentional and should be a conscious decision, not an accident.

### What changes
New file `middleware.go`:

```go
// onMessage wraps h so it only fires for messages in guildID that were not
// sent by the bot itself.
func onMessage(guildID string, h func(*discordgo.Session, *discordgo.MessageCreate)) func(*discordgo.Session, *discordgo.MessageCreate)

// onAnyMessage wraps h so it only fires for messages not sent by the bot itself,
// regardless of guild. Used for cross-server handlers like americanSpellingPolice.
func onAnyMessage(h func(*discordgo.Session, *discordgo.MessageCreate)) func(*discordgo.Session, *discordgo.MessageCreate)
```

Handler bodies lose their guard blocks. Registration in `main.go` becomes:

```go
session.AddHandler(onMessage(Server2985, handleHungry))
session.AddHandler(onMessage(Server2985, handleGetQuote(quotes)))
session.AddHandler(onMessage(Server2985, handleWakeUp(dcaService.GetSound("wake_up.dca"))))
session.AddHandler(onMessage(ServerNXG, handleKit))
session.AddHandler(onMessage(ServerNXG, handleListen(dcaService.GetSound("hey_listen.dca"))))
session.AddHandler(onMessage(ServerNXG, handleBlondie))
session.AddHandler(onAnyMessage(americanSpellingPolice(spellings)))
```

The spelling handler's scope (all guilds vs one guild) becomes explicit and visible in `main.go`.

### Tests to write first (new file `middleware_test.go`)
- `TestOnMessage_WrongGuild` — inner handler must not be called
- `TestOnMessage_BotSelf` — inner handler must not be called
- `TestOnMessage_Fires` — inner handler called with matching guild and non-bot author
- `TestOnAnyMessage_BotSelf` — must not fire
- `TestOnAnyMessage_Fires` — fires regardless of guild

### Files touched
New `middleware.go`, new `middleware_test.go`, `handlers.go`, `main.go`

---

## ✅ Step 6 — Fix the `GiftCodeCommandHandler` Ready anti-pattern *(done)*

### Problem
`GiftCodeCommandHandler` is a Ready handler that, on *every reconnect*, both registers slash commands **and** calls `s.AddHandler(...)` inside the running session. On reconnect, duplicate handlers accumulate silently and fire multiple times per event.

The cleanup logic in `main.go`'s Ready handler only touches global-scope commands (`""`), while `GiftCodeCommandHandler` creates guild-scoped commands — so cleanup never runs on them.

### What changes
1. Expose `KingShot` interaction and message handler methods directly:
   ```go
   func (ks *KingShot) InteractionHandler() func(*discordgo.Session, *discordgo.InteractionCreate)
   func (ks *KingShot) GiftCodeMessageHandler(channelID string) func(*discordgo.Session, *discordgo.MessageCreate)
   ```
2. Register these once in `main.go` alongside all other `session.AddHandler(...)` calls — not inside a Ready callback
3. Move slash command registration to a dedicated helper called once after `session.Open()`:
   ```go
   func registerSlashCommands(s *discordgo.Session, guildID string) error
   ```
4. Delete `GiftCodeCommandHandler` entirely
5. Extend the cleanup in the existing Ready handler to also cover the NXG guild

### Tests to write first
- `TestKingShot_InteractionHandler_Register` — verify routing to `registerPlayer` by checking the deferred response shape (mock Discord HTTP)
- `TestKingShot_InteractionHandler_Code` — verify routing to `addCode` for a user without the r4 role, checking the permission-denied response
- These are integration-level; use `httptest.NewServer` to mock the Discord REST API

### Files touched
`kingshot.go`, `main.go`

---

## ✅ Step 7 — Split into per-server packages *(done)*

### Problem
All handler logic lives in the `main` package. There is no visible boundary between SD and NXG features. Adding a handler to one server requires opening the same file as the other server's handlers.

### Proposed layout

```
discordbot/
  main.go              ← wiring only: parse flags, open session, call Register()
  middleware.go        ← onMessage / onAnyMessage (from Step 5)
  spelling.go          ← LoadSpellingsFromURL, buildSpellingSuggestions (already clean)
  ai/
    handler.go         ← unchanged
  dca/
    dca.go             ← unchanged
  server/
    sd/
      handlers.go      ← handleHungry, handleGetQuote, handleWakeUp + predicates
      handlers_test.go ← predicate tests (moved from handler_predicates_test.go)
      voice.go         ← playAudioToUser (moved from root)
      voice_test.go
    nxg/
      handlers.go      ← handleKit, handleBlondie, handleListen, handleAskGemini + predicates
      handlers_test.go
  kingshot/
    service.go         ← KingShot struct, login, redeemGiftCode, EncodePayload, types
    service_test.go
    store.go           ← loadPlayerIDs, readPlayerFile, writePlayerFile
    store_test.go
    handler.go         ← registerPlayer, addCode, messageHandler, InteractionHandler, GiftCodeMessageHandler
    handler_test.go
```

Each sub-package exposes a single entry point:
```go
// server/sd
func Register(s *discordgo.Session, guildID string, sounds dca.Sounds, spellings map[string]string)

// server/nxg
func Register(s *discordgo.Session, guildID string, aiSvc *ai.Service, sounds dca.Sounds)

// kingshot
func (ks *KingShot) Register(s *discordgo.Session, giftCodeChannelID string)
```

`main.go` knows nothing about handler implementation — it only calls `Register`.

### What was done
- `middleware/` — exported `OnMessage` / `OnAnyMessage`; tests moved here
- `voice/` — exported `PlayAudioToUser`; tests moved here
- `server/sd/` — SD handlers (getQuote, hungry, wakeUp), predicates, `Register()`
- `server/nxg/` — NXG handlers (kit, blondie, listen, askGemini), predicates, `Register()`
- `kingshot/` — KingShot service moved here, `Register()` entry point added
- `main.go` — wiring only: parse flags, build deps, call `sd.Register`, `nxg.Register`, `ks.Register`

### Files touched
All files — final structural step.

---

## Issue Backlog

Items resolved as a side-effect of Steps 1–2 are struck through.

| Issue | Location | Fix |
|---|---|---|
| `r4RoleId` hardcoded | `kingshot.go:42` | Make it a configurable flag passed into `NewKingShot` |
| KingShot API `Key` in source | `kingshot.go:29` | Move to env var / flag |
| Gemini model name hardcoded | `ai/handler.go:34` | Named const or config field on `Service` |
| ~~`1900` magic number~~ | ~~`kingshot.go`~~ | ~~`const discordMaxMessageLen`~~ — done in Step 1 |
| ~~`chunkMessage` not extracted~~ | ~~`addCode()`~~ | ~~done in Step 1~~ |
| ~~Hardcoded `"1423406563850190850"` in `GiftCodeCommandHandler`~~ | ~~`kingshot.go:96`~~ | ~~Fixed to use `ServerNXG` in Step 1~~ |
| `loadSound` prints to stdout | `dca/dca.go:47,63,67` | Replace `fmt.Println` with the injected logger |
| `EncodePayload` mutates its argument | `kingshot.go` | Return a new map; don't modify the caller's data |

---

## Acceptance Criteria (every step)

1. `go test -race ./...` — no failures, no race warnings
2. `go vet ./...` — clean
3. No change in observable bot behaviour
4. Each step committed independently so it can be reviewed and reverted in isolation
