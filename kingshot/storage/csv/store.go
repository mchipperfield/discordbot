package csv

import (
	"encoding/csv"
	"os"
	"sync"
	"syscall"
)

// Store persists player registrations in a CSV file.
type Store struct {
	path string
	mu   sync.Mutex
}

// New returns a CSV-backed player store.
func New(path string) *Store {
	return &Store{path: path}
}

// ListPlayerIDs returns all player IDs in file order.
func (s *Store) ListPlayerIDs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.path, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		out = append(out, record[0])
	}
	return out, nil
}

// GetDiscordID returns the discord ID currently associated with playerID.
func (s *Store) GetDiscordID(playerID string) (discordID string, found bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.path, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		return "", false, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return "", false, err
	}

	for _, record := range records {
		if len(record) == 0 || record[0] != playerID {
			continue
		}
		if len(record) > 1 {
			return record[1], true, nil
		}
		return "", true, nil
	}
	return "", false, nil
}

// Upsert inserts or updates a registration keyed by player ID.
func (s *Store) Upsert(playerID, discordID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.OpenFile(s.path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	normalized := make([][]string, 0, len(records))
	for _, row := range records {
		if len(row) == 0 {
			continue
		}
		rowDiscordID := ""
		if len(row) > 1 {
			rowDiscordID = row[1]
		}
		normalized = append(normalized, []string{row[0], rowDiscordID})
	}
	records = normalized

	updated := false
	for i := range records {
		if records[i][0] == playerID {
			records[i] = []string{playerID, discordID}
			updated = true
			break
		}
	}
	if !updated {
		records = append(records, []string{playerID, discordID})
	}

	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	for _, row := range records {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
