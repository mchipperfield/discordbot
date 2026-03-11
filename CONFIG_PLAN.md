# CONFIG_PLAN.md

## Problem

Every new handler that needs a dependency adds another positional parameter to `Register()`.
`nxg.Register` already has 4 parameters. This does not scale — positional args are hard to
read, easy to get in the wrong order, and every addition forces a signature change and a
`main.go` edit.

## Goal

Adding a new feature to a server should require:
1. Adding a field to that server's `Config` struct
2. Using the field inside the new handler
3. Populating the field in `main.go`

No signature changes. No reordering. No breaking other callers.

---

## Changes

### 1. `server/sd` — introduce `Config`

**New type:**
```go
type Config struct {
    GuildID     string
    WakeUpSound [][]byte // nil = wake-up gag disabled
}
```

**Updated signature:**
```go
func Register(s *discordgo.Session, cfg Config)
```

**Before (`main.go`):**
```go
sd.Register(session, *serverId, dcaService.GetSound("wake_up.dca"))
```

**After (`main.go`):**
```go
sd.Register(session, sd.Config{
    GuildID:     *serverId,
    WakeUpSound: dcaService.GetSound("wake_up.dca"),
})
```

---

### 2. `server/nxg` — introduce `Config`

**New type:**
```go
type Config struct {
    GuildID     string
    ListenSound [][]byte    // nil = listen gag disabled
    AI          *ai.Service // nil = /ask command disabled
}
```

**Updated signature:**
```go
func Register(s *discordgo.Session, cfg Config)
```

**Before (`main.go`):**
```go
nxg.Register(session, *nxgID, dcaService.GetSound("hey_listen.dca"), aiService)
```

**After (`main.go`):**
```go
nxg.Register(session, nxg.Config{
    GuildID:     *nxgID,
    ListenSound: dcaService.GetSound("hey_listen.dca"),
    AI:          aiService,
})
```

---

### 3. `kingshot` — move `giftCodeChannelID` into `NewKingShot`

`giftCodeChannelID` is fixed configuration, not a runtime option. It belongs at
construction time alongside `playerIDFile`, not passed again at `Register()`.

**Before:**
```go
ks := kingshot.NewKingShot(*playerIDFile)
ks.Register(session, *giftCodeChannelID)
```

**After:**
```go
ks := kingshot.NewKingShot(*playerIDFile, *giftCodeChannelID)
ks.Register(session)
```

`KingShot` struct gains a `giftCodeChannelID string` field. `NewKingShot` accepts it as a
second parameter. `Register` drops its parameter entirely.

---

## What does NOT change

- The internal handler functions — they don't know about `Config`
- The `middleware.OnMessage` wiring inside `Register`
- Test behaviour — predicate tests and smoke tests stay as-is; only the `Register` call
  sites in `_test.go` files need updating to pass a `Config` struct

---

## Acceptance criteria

- `go test -race ./...` passes clean
- `main.go` registration block reads as a named-field config literal for each server
- Adding a new field to `nxg.Config` or `sd.Config` requires zero changes to `Register`'s
  signature
