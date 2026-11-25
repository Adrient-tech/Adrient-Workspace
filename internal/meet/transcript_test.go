package meet

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCleanSegment(t *testing.T) {
	if _, ok := CleanSegment("  \n "); ok {
		t.Fatal("blank accepted")
	}
	got, _ := CleanSegment("  hello \n  world  ")
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
	long, _ := CleanSegment(strings.Repeat("x", 5000))
	if len([]rune(long)) != MaxSegmentRunes {
		t.Fatalf("len %d", len([]rune(long)))
	}
}

func TestTranscriptText(t *testing.T) {
	at := time.Date(2026, 11, 19, 14, 3, 0, 0, time.UTC)
	out := TranscriptText("Standup", []Segment{{Speaker: "Maya", Text: "hi", At: at}}, time.UTC)
	if !strings.Contains(out, "[14:03] Maya: hi") || !strings.HasPrefix(out, "Transcript: Standup") {
		t.Fatal(out)
	}
	if !strings.Contains(TranscriptText("x", nil, nil), "no speech") {
		t.Fatal("empty case")
	}
}

func TestTranscriptStore(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	room, err := repo.Create(ctx, orgID, userID, "Standup")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.AddSegment(ctx, orgID, room.ID, userID, "Tester", "first"); !ok || err != nil {
		t.Fatalf("add %v %v", ok, err)
	}
	_, _ = repo.AddSegment(ctx, orgID, room.ID, userID, "Tester", "second")
	segs, err := repo.Segments(ctx, orgID, room.ID)
	if err != nil || len(segs) != 2 || segs[0].Text != "first" {
		t.Fatalf("segments %+v %v", segs, err)
	}
	// Another org sees nothing.
	if other, _ := repo.Segments(ctx, "00000000-0000-0000-0000-000000000009", room.ID); len(other) != 0 {
		t.Fatal("cross-org read")
	}
	if err := repo.ClearSegments(ctx, orgID, room.ID); err != nil {
		t.Fatal(err)
	}
	if segs, _ = repo.Segments(ctx, orgID, room.ID); len(segs) != 0 {
		t.Fatal("not cleared")
	}
}
