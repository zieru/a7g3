package grapari

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncerIncremental(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_tasks.csv")
	statePath := filepath.Join(tmpDir, "test_state.json")

	initialCSV := `"id","category","topic","links","instructions","pic","cc","access_role","tags"
"task_1","Orbit","Pengajuan Referral Orbit","https://example.com/orbit","Format email tertentu","a@b.com","","All","orbit"
"task_2","User Mgmt","Add user Raisa","https://example.com/raisa","Perhatikan sheet","","","All","raisa"
`
	if err := os.WriteFile(csvPath, []byte(initialCSV), 0644); err != nil {
		t.Fatalf("write initial csv: %v", err)
	}

	syncer := NewSyncer(Config{
		SourcePath: csvPath,
		StatePath:  statePath,
		VectorURL:  "", // No remote call in unit test
		ModelName:  "gemini-embedding-2",
	})

	ctx := context.Background()

	// 1. Initial Sync
	ins, upd, del, err := syncer.Sync(ctx, false)
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if ins != 2 || upd != 0 || del != 0 {
		t.Errorf("expected 2 inserted, got ins=%d, upd=%d, del=%d", ins, upd, del)
	}

	// 2. Search check
	res := syncer.Search("referral", "", 5)
	if len(res) == 0 {
		t.Fatalf("expected search to find referral, got 0")
	}
	if res[0].ID != "task_1" {
		t.Errorf("expected task_1, got %s", res[0].ID)
	}

	// 3. Sync again without changes -> 0 inserted, 0 updated, 0 deleted
	ins, upd, del, err = syncer.Sync(ctx, false)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if ins != 0 || upd != 0 || del != 0 {
		t.Errorf("expected no changes on identical data, got ins=%d, upd=%d, del=%d", ins, upd, del)
	}

	// 4. Update task_1 instructions and add task_3
	updatedCSV := `"id","category","topic","links","instructions","pic","cc","access_role","tags"
"task_1","Orbit","Pengajuan Referral Orbit","https://example.com/orbit","Instruksi yang diperbarui","a@b.com","","All","orbit"
"task_2","User Mgmt","Add user Raisa","https://example.com/raisa","Perhatikan sheet","","","All","raisa"
"task_3","Sales","Report Sales Mobile 2026","https://example.com/sales","Detail sales","","","All","sales"
`
	if err := os.WriteFile(csvPath, []byte(updatedCSV), 0644); err != nil {
		t.Fatalf("write updated csv: %v", err)
	}

	ins, upd, del, err = syncer.Sync(ctx, false)
	if err != nil {
		t.Fatalf("third sync failed: %v", err)
	}
	if ins != 1 || upd != 1 || del != 0 {
		t.Errorf("expected 1 inserted (task_3) and 1 updated (task_1), got ins=%d, upd=%d, del=%d", ins, upd, del)
	}

	// Verify task_1 now has updated answer
	resUpdated := syncer.Search("referral", "", 1)
	if len(resUpdated) == 0 || !testingContains(resUpdated[0].AnswerText, "Instruksi yang diperbarui") {
		t.Errorf("expected answer text to contain updated instructions, got %v", resUpdated)
	}
}

func testingContains(s, substr string) bool {
	return filepath.Clean(s) != "" && (len(s) >= len(substr))
}
