package csv

import (
	"encoding/csv"
	"io"
	"os"
	"sync"
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

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil && err != io.EOF {
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

	records, err := csv.NewReader(file).ReadAll()
	if err != nil && err != io.EOF {
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

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil && err != io.EOF {
		return err
	}

	updated := false
	for i := range records {
		if len(records[i]) == 0 {
			continue
		}
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
		if len(row) == 0 {
			continue
		}
		if len(row) == 1 {
			row = []string{row[0], ""}
		}
		if err := writer.Write(row[:2]); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
