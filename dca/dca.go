package dca

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mchipperfield/gocore/log"
)

type Service struct {
	sounds map[string][][]byte
}

// NewService creates a new DCA service by loading all .dca files in the current directory.
// TODO: Pass in directory path?
func NewService(l log.Logger) (*Service, error) {
	filenames, err := filepath.Glob("*.dca")
	if err != nil {
		return nil, err
	}
	sounds := make(map[string][][]byte, len(filenames))
	for _, filename := range filenames {
		opus, err := loadSound(filename)
		if err != nil {
			// error loading one sound shouldn't prevent others from loading
			l.Log("failed to load sound", "error", err, "file", filename)
			continue
		}
		sounds[filename] = opus
	}
	return &Service{
		sounds: sounds,
	}, nil
}

func (s *Service) GetSound(name string) [][]byte {
	return s.sounds[name]
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
