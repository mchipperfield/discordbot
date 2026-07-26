package kingshot

// PlayerStore abstracts player registration storage.
type PlayerStore interface {
	ListPlayerIDs() ([]string, error)
	GetDiscordID(playerID string) (discordID string, found bool, err error)
	Upsert(playerID, discordID string) error
}
