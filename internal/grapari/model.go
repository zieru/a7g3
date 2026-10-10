package grapari

// Task represents a single GraPARI operational knowledge entry or task.
type Task struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Topic        string `json:"topic"`
	Links        string `json:"links"`
	Instructions string `json:"instructions"`
	PIC          string `json:"pic"`
	CC           string `json:"cc"`
	AccessRole   string `json:"access_role"`
	Tags         string `json:"tags"`
	ContentHash  string `json:"content_hash"`
}

// SyncState tracks the last synchronized hashes to enable smart incremental updates.
type SyncState struct {
	SourceFile string            `json:"source_file"`
	LastSynced string            `json:"last_synced"`
	Hashes     map[string]string `json:"hashes"` // task_id -> sha256
}

// VectorItem represents the format sent to the embedding service.
type VectorItem struct {
	ID       string                 `json:"id"`
	Category string                 `json:"category"`
	Topic    string                 `json:"topic"`
	Text     string                 `json:"text"`
	Metadata map[string]interface{} `json:"metadata"`
}

// VectorPayload is the request body sent to goaiassistant HTTP endpoint.
type VectorPayload struct {
	Action    string       `json:"action"` // "upsert" or "delete"
	Model     string       `json:"model"`  // default: gemini-embedding-2
	Namespace string       `json:"namespace"`
	Items     []VectorItem `json:"items"`
}

// VectorSearchResult represents an item returned from semantic retrieval.
type VectorSearchResult struct {
	ID         string  `json:"id"`
	Category   string  `json:"category"`
	Topic      string  `json:"topic"`
	AnswerText string  `json:"answer_text"`
	Score      float64 `json:"score,omitempty"`
}
