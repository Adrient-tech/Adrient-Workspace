package meet

import (
	"context"
	"testing"
	"time"
)

func TestRepository_StateAndRecordings(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	room, err := repo.Create(ctx, orgID, userID, "Sync")
	if err != nil {
		t.Fatal(err)
	}
	if p, err := repo.LoadState(ctx, room.ID); err != nil || p != nil {
		t.Fatalf("empty: %v %v", p, err)
	}
	in := persistedState{HostID: userID, Locked: true, WaitingRoom: true, EgressID: "EG1",
		Banned: []string{"b1"}, Admitted: []string{"a1", "a2"}, Denied: []string{"d1~x"},
		Waiting: []waiter{{Identity: "w1~c", Name: "Wendy", Since: time.Now()}}}
	if err := repo.SaveState(ctx, room.ID, in); err != nil {
		t.Fatal(err)
	}
	out, err := repo.LoadState(ctx, room.ID)
	if err != nil || out == nil {
		t.Fatalf("load: %v %v", out, err)
	}
	if !out.Locked || !out.WaitingRoom || out.EgressID != "EG1" || out.HostID != userID ||
		len(out.Banned) != 1 || len(out.Admitted) != 2 || len(out.Denied) != 1 || len(out.Waiting) != 1 || out.Waiting[0].Name != "Wendy" {
		t.Fatalf("round trip: %+v", out)
	}
	// overwrite shrinks the sets
	in.Banned, in.Waiting, in.Locked = nil, nil, false
	if err := repo.SaveState(ctx, room.ID, in); err != nil {
		t.Fatal(err)
	}
	out, _ = repo.LoadState(ctx, room.ID)
	if out.Locked || len(out.Banned) != 0 || len(out.Waiting) != 0 {
		t.Fatalf("shrink: %+v", out)
	}

	job := RecordingJob{EgressID: "EG1", OrgID: orgID, RoomID: room.ID, StartedBy: userID, Key: "k.mp4", StartedAt: time.Now()}
	if err := repo.StartRecording(ctx, job); err != nil {
		t.Fatal(err)
	}
	pend, err := repo.PendingRecordings(ctx)
	if err != nil || len(pend) != 1 || pend[0].RoomName != "Sync" || pend[0].StarterNm != "Tester" {
		t.Fatalf("pending: %+v %v", pend, err)
	}
	if err := repo.FinishRecording(ctx, "EG1", "failed", ""); err != nil {
		t.Fatal(err)
	}
	if pend, _ = repo.PendingRecordings(ctx); len(pend) != 0 {
		t.Fatalf("still pending: %+v", pend)
	}
	if err := repo.DeleteState(ctx, room.ID); err != nil {
		t.Fatal(err)
	}
}
