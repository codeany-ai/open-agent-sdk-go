package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a history log entry.
type Entry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId,omitempty"`
}

// Store manages conversation history persistence.
type Store struct {
	mu       sync.Mutex
	filePath string
	buffer   []Entry
}

// NewStore creates a new history store.
func NewStore(configDir string) *Store {
	return &Store{
		filePath: filepath.Join(configDir, "history.jsonl"),
	}
}

// Add adds an entry to the history.
func (s *Store) Add(entry Entry) {
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixMilli()
	}

	s.mu.Lock()
	s.buffer = append(s.buffer, entry)
	s.mu.Unlock()

	// Flush async
	go s.flush()
}

// AddSimple adds a simple text entry.
func (s *Store) AddSimple(display, project, sessionID string) {
	s.Add(Entry{
		Display:   display,
		Timestamp: time.Now().UnixMilli(),
		Project:   project,
		SessionID: sessionID,
	})
}

// GetHistory returns history entries for a project, newest first.
func (s *Store) GetHistory(project string, maxEntries int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var allEntries []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if project == "" || entry.Project == project {
			allEntries = append(allEntries, entry)
		}
	}

	// Reverse (newest first)
	for i, j := 0, len(allEntries)-1; i < j; i, j = i+1, j-1 {
		allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
	}

	// Deduplicate by display text
	seen := make(map[string]bool)
	var deduped []Entry
	for _, e := range allEntries {
		if seen[e.Display] {
			continue
		}
		seen[e.Display] = true
		deduped = append(deduped, e)
		if maxEntries > 0 && len(deduped) >= maxEntries {
			break
		}
	}

	return deduped, nil
}

// flush writes buffered entries to disk.
func (s *Store) flush() {
	s.mu.Lock()
	if len(s.buffer) == 0 {
		s.mu.Unlock()
		return
	}
	entries := s.buffer
	s.buffer = nil
	s.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		file.Write(append(data, '\n'))
	}
}

// RemoveLast removes the most recent history entry.
func (s *Store) RemoveLast() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	// Find last newline
	lastNewline := -1
	for i := len(data) - 2; i >= 0; i-- {
		if data[i] == '\n' {
			lastNewline = i
			break
		}
	}

	if lastNewline < 0 {
		// Only one entry, truncate file
		return os.WriteFile(s.filePath, nil, 0644)
	}

	return os.WriteFile(s.filePath, data[:lastNewline+1], 0644)
}
