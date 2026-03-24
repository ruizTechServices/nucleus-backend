package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileSink struct {
	mu   sync.Mutex
	file *os.File
}

func NewFileSink(path string) (*FileSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	return &FileSink{file: file}, nil
}

func (s *FileSink) Append(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.file.Write(append(encoded, '\n')); err != nil {
		return err
	}

	return s.file.Sync()
}

func (s *FileSink) Close() error {
	if s == nil || s.file == nil {
		return nil
	}

	return s.file.Close()
}

func LoadEvents(path string) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, scanner.Err()
}
