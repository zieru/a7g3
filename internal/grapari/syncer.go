package grapari

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Syncer handles parsing tasks and incrementally sending updates to the vector service.
type Syncer struct {
	mu         sync.RWMutex
	sourcePath string
	statePath  string
	vectorURL  string
	modelName  string
	httpClient *http.Client
	tasks      map[string]Task
	state      SyncState
}

// Config holds the syncer configuration parameters.
type Config struct {
	SourcePath string
	StatePath  string
	VectorURL  string
	ModelName  string
}

// NewSyncer initializes a new Syncer instance.
func NewSyncer(cfg Config) *Syncer {
	if cfg.ModelName == "" {
		cfg.ModelName = "gemini-embedding-2"
	}
	if cfg.StatePath == "" {
		cfg.StatePath = ".grapari_sync_state.json"
	}
	return &Syncer{
		sourcePath: cfg.SourcePath,
		statePath:  cfg.StatePath,
		vectorURL:  cfg.VectorURL,
		modelName:  cfg.ModelName,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		tasks:      make(map[string]Task),
		state: SyncState{
			Hashes: make(map[string]string),
		},
	}
}

// LoadState loads the sync state from disk if present.
func (s *Syncer) LoadState() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = SyncState{Hashes: make(map[string]string)}
			return nil
		}
		return err
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Hashes == nil {
		state.Hashes = make(map[string]string)
	}
	s.state = state
	return nil
}

// SaveState writes the current sync state to disk.
func (s *Syncer) SaveState() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath, data, 0644)
}

// ReadTasks reads tasks from the configured source file (CSV or delimited).
func (s *Syncer) ReadTasks(filePath string) ([]Task, error) {
	if filePath == "" {
		filePath = s.sourcePath
	}
	cleanPath := filepath.Clean(filePath)
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("open source file %q: %w", cleanPath, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, h := range headers {
		colIdx[strings.ToLower(strings.TrimSpace(h))] = i
	}

	var tasks []Task
	rowNum := 1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read line %d: %w", rowNum, err)
		}
		rowNum++

		getVal := func(name string) string {
			if idx, ok := colIdx[name]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx])
			}
			return ""
		}

		id := getVal("id")
		if id == "" {
			id = getVal("task_id")
		}
		topic := getVal("topic")
		if topic == "" {
			topic = getVal("title")
		}
		if id == "" && topic != "" {
			// Auto generate clean ID from topic
			id = sanitizeID(topic)
		}
		if id == "" {
			id = fmt.Sprintf("task_%d", rowNum)
		}

		t := Task{
			ID:           id,
			Category:     getVal("category"),
			Topic:        topic,
			Links:        getVal("links"),
			Instructions: getVal("instructions"),
			PIC:          getVal("pic"),
			CC:           getVal("cc"),
			AccessRole:   getVal("access_role"),
			Tags:         getVal("tags"),
		}
		t.ContentHash = computeHash(t)
		tasks = append(tasks, t)
	}

	return tasks, nil
}

// Sync performs the smart incremental synchronization.
func (s *Syncer) Sync(ctx context.Context, forceAll bool) (inserted int, updated int, deleted int, err error) {
	if err := s.LoadState(); err != nil {
		return 0, 0, 0, fmt.Errorf("load state: %w", err)
	}

	tasks, err := s.ReadTasks(s.sourcePath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read tasks: %w", err)
	}

	s.mu.Lock()
	currentMap := make(map[string]Task)
	for _, t := range tasks {
		currentMap[t.ID] = t
		s.tasks[t.ID] = t
	}
	s.mu.Unlock()

	var toUpsert []VectorItem

	for _, t := range tasks {
		oldHash, exists := s.state.Hashes[t.ID]
		if !exists || forceAll {
			toUpsert = append(toUpsert, taskToVectorItem(t))
			inserted++
		} else if oldHash != t.ContentHash {
			toUpsert = append(toUpsert, taskToVectorItem(t))
			updated++
		}
	}

	// Detect deleted tasks
	var deletedIDs []string
	for oldID := range s.state.Hashes {
		if _, stillExists := currentMap[oldID]; !stillExists {
			deletedIDs = append(deletedIDs, oldID)
			deleted++
		}
	}

	// Send to vector service if vectorURL is configured
	if s.vectorURL != "" {
		if len(toUpsert) > 0 {
			payload := VectorPayload{
				Action:    "upsert",
				Model:     s.modelName,
				Namespace: "grapari_sop",
				Items:     toUpsert,
			}
			if err := s.sendHTTP(ctx, payload); err != nil {
				// Log error but don't fail local sync state so search_grapari_knowledge remains operational offline
				fmt.Fprintf(os.Stderr, "[grapari-syncer] warning: could not push vectors to %s: %v\n", s.vectorURL, err)
			}
		}

		if len(deletedIDs) > 0 {
			var delItems []VectorItem
			for _, id := range deletedIDs {
				delItems = append(delItems, VectorItem{ID: id})
			}
			payload := VectorPayload{
				Action:    "delete",
				Model:     s.modelName,
				Namespace: "grapari_sop",
				Items:     delItems,
			}
			_ = s.sendHTTP(ctx, payload)
		}
	}

	// Update local state hashes
	s.mu.Lock()
	for _, t := range tasks {
		s.state.Hashes[t.ID] = t.ContentHash
	}
	for _, id := range deletedIDs {
		delete(s.state.Hashes, id)
	}
	s.state.SourceFile = s.sourcePath
	s.state.LastSynced = time.Now().Format(time.RFC3339)
	s.mu.Unlock()

	if err := s.SaveState(); err != nil {
		return inserted, updated, deleted, fmt.Errorf("save state: %w", err)
	}

	return inserted, updated, deleted, nil
}

// Search retrieves local matching tasks (hybrid local keyword / fallback search).
func (s *Syncer) Search(query string, category string, limit int) []VectorSearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 3
	}

	q := strings.ToLower(strings.TrimSpace(query))
	cat := strings.ToLower(strings.TrimSpace(category))
	tokens := strings.Fields(q)

	type scored struct {
		task  Task
		score float64
	}
	var matches []scored

	for _, t := range s.tasks {
		if cat != "" && !strings.Contains(strings.ToLower(t.Category), cat) {
			continue
		}

		textPool := strings.ToLower(fmt.Sprintf("%s %s %s %s %s", t.Topic, t.Category, t.Instructions, t.Tags, t.AccessRole))
		score := 0.0

		if strings.Contains(textPool, q) {
			score += 10.0
		}
		for _, token := range tokens {
			if strings.Contains(textPool, token) {
				score += 2.0
			}
		}

		if score > 0 {
			matches = append(matches, scored{task: t, score: score})
		}
	}

	// Sort desc by score
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].score > matches[i].score {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	var results []VectorSearchResult
	for i := 0; i < len(matches) && i < limit; i++ {
		t := matches[i].task
		answer := formatAnswer(t)
		results = append(results, VectorSearchResult{
			ID:         t.ID,
			Category:   t.Category,
			Topic:      t.Topic,
			AnswerText: answer,
			Score:      matches[i].score,
		})
	}

	return results
}

// GetTasks returns a copy of loaded tasks.
func (s *Syncer) GetTasks() []Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		list = append(list, t)
	}
	return list
}

func (s *Syncer) sendHTTP(ctx context.Context, payload VectorPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.vectorURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vector service returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func taskToVectorItem(t Task) VectorItem {
	fullText := fmt.Sprintf("[%s] %s\nInstruksi: %s\nLink: %s", t.Category, t.Topic, t.Instructions, t.Links)
	if t.PIC != "" {
		fullText += fmt.Sprintf("\nPIC: %s", t.PIC)
	}
	if t.CC != "" {
		fullText += fmt.Sprintf("\nCC: %s", t.CC)
	}
	if t.AccessRole != "" {
		fullText += fmt.Sprintf("\nRole: %s", t.AccessRole)
	}

	return VectorItem{
		ID:       t.ID,
		Category: t.Category,
		Topic:    t.Topic,
		Text:     fullText,
		Metadata: map[string]interface{}{
			"links":       t.Links,
			"access_role": t.AccessRole,
			"pic":         t.PIC,
			"cc":          t.CC,
		},
	}
}

func formatAnswer(t Task) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s** (%s)\n", t.Topic, t.Category))
	if t.Instructions != "" {
		sb.WriteString(fmt.Sprintf("• **Petunjuk**: %s\n", t.Instructions))
	}
	if t.Links != "" {
		sb.WriteString(fmt.Sprintf("• **Link**: %s\n", t.Links))
	}
	if t.PIC != "" {
		sb.WriteString(fmt.Sprintf("• **PIC / Tujuan**: %s\n", t.PIC))
	}
	if t.CC != "" {
		sb.WriteString(fmt.Sprintf("• **CC**: %s\n", t.CC))
	}
	if t.AccessRole != "" {
		sb.WriteString(fmt.Sprintf("• **Akses**: %s\n", t.AccessRole))
	}
	return strings.TrimSpace(sb.String())
}

func computeHash(t Task) string {
	h := sha256.New()
	h.Write([]byte(t.Category + "|" + t.Topic + "|" + t.Links + "|" + t.Instructions + "|" + t.PIC + "|" + t.CC + "|" + t.AccessRole + "|" + t.Tags))
	return hex.EncodeToString(h.Sum(nil))
}

func sanitizeID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '/' {
			b.WriteRune('_')
		}
	}
	res := strings.Trim(b.String(), "_")
	if len(res) > 50 {
		res = res[:50]
	}
	return res
}
