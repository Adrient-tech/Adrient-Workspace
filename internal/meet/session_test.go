package meet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatDurationAndBody(t *testing.T) {
	cases := map[int]string{0: "0s", 45: "45s", 723: "12m 03s", 3900: "1h 05m"}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%d)=%q want %q", in, got, want)
		}
	}
	info := EndInfo{Room: Room{Name: "Standup"}, DurationS: 90, Participants: []string{"a", "b"}}
	if got := EndedBody(info); got != "Standup ended after 1m 30s · 2 participants" {
		t.Errorf("body = %q", got)
	}
	info.Participants = []string{"a"}
	if !strings.Contains(EndedBody(info), "1 participant") || strings.Contains(EndedBody(info), "1 participants") {
		t.Errorf("singular: %q", EndedBody(info))
	}
}

func TestRoomEndedAndDuration(t *testing.T) {
	st := time.Now().Add(-10 * time.Minute)
	en := st.Add(7*time.Minute + 5*time.Second)
	r := Room{SessionStartedAt: &st}
	if r.Ended() || r.DurationS() != 0 {
		t.Fatal("running room must not be ended")
	}
	r.EndedAt = &en
	if !r.Ended() || r.DurationS() != 425 {
		t.Fatalf("ended=%v dur=%d", r.Ended(), r.DurationS())
	}
}

func TestEndedEventShape(t *testing.T) {
	en := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ev := EndedEvent(EndInfo{Room: Room{ID: "r1", Code: "abc-defg-hij", Name: "N", EndedReason: EndedHost},
		EndedAt: en, DurationS: 60, Participants: []string{"u1", "u2", "u3"}})
	if ev["type"] != "meet_ended" || ev["code"] != "abc-defg-hij" || ev["participant_count"] != 3 ||
		ev["duration_s"] != 60 || ev["reason"] != EndedHost || ev["ended_at"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("event: %v", ev)
	}
}

// A page refresh (leave then rejoin inside the grace period) must not end the
// meeting; a real departure must, exactly once.
func TestHubOnEmptyGrace(t *testing.T) {
	h := NewHub()
	h.EmptyGrace = 60 * time.Millisecond
	var mu sync.Mutex
	fired := 0
	h.OnEmpty = func(string) { mu.Lock(); fired++; mu.Unlock() }

	cr := h.getOrCreate("room")
	cr.mu.Lock()
	cr.peers["p1"] = &peer{id: "p1"}
	cr.mu.Unlock()
	h.scheduleEmpty("room") // someone "left" but p1 is still here
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	if fired != 0 {
		t.Fatalf("fired with a participant present: %d", fired)
	}
	mu.Unlock()

	cr.mu.Lock()
	delete(cr.peers, "p1")
	cr.mu.Unlock()
	h.cleanup("room", cr)
	h.scheduleEmpty("room")
	// rejoin inside the grace period
	cr2 := h.getOrCreate("room")
	cr2.mu.Lock()
	cr2.peers["p1b"] = &peer{id: "p1b"}
	cr2.mu.Unlock()
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	if fired != 0 {
		t.Fatalf("refresh ended the meeting: %d", fired)
	}
	mu.Unlock()

	cr2.mu.Lock()
	delete(cr2.peers, "p1b")
	cr2.mu.Unlock()
	h.cleanup("room", cr2)
	h.scheduleEmpty("room")
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if fired != 1 {
		t.Fatalf("expected one end, got %d", fired)
	}
}

func TestSessionLifecycle(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	room, err := repo.Create(ctx, orgID, userID, "Sync")
	if err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := repo.EndSession(ctx, room.ID, EndedEmpty); ok {
		t.Fatal("a meeting nobody joined cannot end")
	}
	if ok, err := repo.BeginSession(ctx, room.ID, userID); err != nil || !ok {
		t.Fatalf("begin: %v %v", ok, err)
	}
	_, _ = repo.BeginSession(ctx, room.ID, userID) // same user again: still one participant
	_, _ = repo.BeginSession(ctx, room.ID, "")     // guests are not counted

	var published []map[string]any
	var notified []string
	f := &Finisher{Repo: repo,
		Publish: func(org string, ev map[string]any) { published = append(published, ev) },
		Notify:  func(_ context.Context, _, uid string, _ EndInfo) { notified = append(notified, uid) },
	}
	time.Sleep(1100 * time.Millisecond)
	info, ok := f.Finish(ctx, room.ID, EndedHost)
	if !ok || info.DurationS < 1 || len(info.Participants) != 1 {
		t.Fatalf("finish: ok=%v info=%+v", ok, info)
	}
	if len(published) != 1 || published[0]["type"] != "meet_ended" || published[0]["participant_count"] != 1 {
		t.Fatalf("published: %v", published)
	}
	if len(notified) != 1 || notified[0] != userID {
		t.Fatalf("notified: %v", notified)
	}
	// Second trigger (room empties after the host ended it) is a no-op.
	if _, ok := f.Finish(ctx, room.ID, EndedEmpty); ok || len(published) != 1 {
		t.Fatal("meeting ended twice")
	}
	got, _ := repo.Get(ctx, orgID, room.ID)
	if !got.Ended() || got.EndedReason != EndedHost {
		t.Fatalf("room: %+v", got)
	}
	// Joining an ended meeting is refused until it is restarted.
	if ok, _ := repo.BeginSession(ctx, room.ID, userID); ok {
		t.Fatal("joined an ended meeting")
	}
	if err := repo.RestartSession(ctx, orgID, room.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.BeginSession(ctx, room.ID, userID); !ok {
		t.Fatal("cannot join after restart")
	}
	got, _ = repo.Get(ctx, orgID, room.ID)
	if got.Ended() || len(got.ParticipantIDs) != 1 {
		t.Fatalf("after restart: %+v", got)
	}
}

// The starter leaves but others remain: the meeting must NOT end. Only when
// the last one goes does it end.
func TestStarterLeavesOthersStay(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	room, _ := repo.Create(ctx, orgID, userID, "Long")
	_, _ = repo.BeginSession(ctx, room.ID, userID)

	h := NewHub()
	h.EmptyGrace = 40 * time.Millisecond
	ended := make(chan string, 2)
	f := &Finisher{Repo: repo}
	h.OnEmpty = func(id string) {
		if _, ok := f.Finish(ctx, id, EndedEmpty); ok {
			ended <- id
		}
	}
	cr := h.getOrCreate(room.ID)
	cr.mu.Lock()
	cr.peers["other"] = &peer{id: "other"} // still in the call
	cr.mu.Unlock()
	h.scheduleEmpty(room.ID) // starter just left
	select {
	case <-ended:
		t.Fatal("meeting ended while a participant remained")
	case <-time.After(200 * time.Millisecond):
	}
	cr.mu.Lock()
	delete(cr.peers, "other")
	cr.mu.Unlock()
	h.cleanup(room.ID, cr)
	h.scheduleEmpty(room.ID)
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("meeting did not end after the last participant left")
	}
}

func TestRestartEndpoint(t *testing.T) {
	pool, orgID, userID := setupDB(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	room, _ := repo.Create(ctx, orgID, userID, "R")
	_, _ = repo.BeginSession(ctx, room.ID, userID)
	_, _, _ = repo.EndSession(ctx, room.ID, EndedEmpty)

	call := func(uid string, method, path string) *httptest.ResponseRecorder {
		h := NewCodesHandler(repo,
			func(*http.Request) (string, bool) { return orgID, true },
			func(*http.Request) (string, bool) { return uid, true })
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	w := call(userID, "GET", "/api/v1/meet/codes/"+room.Code)
	if !strings.Contains(w.Body.String(), `"ended":true`) || !strings.Contains(w.Body.String(), `"participant_count":1`) {
		t.Fatalf("resolve: %s", w.Body.String())
	}
	if w := call("00000000-0000-0000-0000-000000000001", "POST", "/api/v1/meet/codes/"+room.Code+"/restart"); w.Code != 403 {
		t.Fatalf("non-host restart: %d", w.Code)
	}
	w = call(userID, "POST", "/api/v1/meet/codes/"+room.Code+"/restart")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"ended":true`) {
		t.Fatalf("restart: %d %s", w.Code, w.Body.String())
	}
}
