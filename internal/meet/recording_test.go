package meet

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type memBlobs struct {
	mu   sync.Mutex
	data map[string]string
}

func (m *memBlobs) Get(_ context.Context, k string) (io.ReadCloser, string, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[k]
	if !ok {
		return nil, "", 0, errors.New("missing")
	}
	return io.NopCloser(strings.NewReader(v)), "video/mp4", int64(len(v)), nil
}
func (m *memBlobs) Delete(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, k)
	return nil
}
func (m *memBlobs) Put(_ context.Context, k, _ string, _ int64, b io.Reader) error {
	d, _ := io.ReadAll(b)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[k] = string(d)
	return nil
}

type fakeDrive struct {
	folders map[string]string
	files   []string
}

func (f *fakeDrive) NewKey() (string, error) { return "blobs/aa/bb", nil }
func (f *fakeDrive) Folder(_ context.Context, _, owner, name string) (string, error) {
	if f.folders == nil {
		f.folders = map[string]string{}
	}
	f.folders[owner+name] = "folder-1"
	return "folder-1", nil
}
func (f *fakeDrive) AddFile(_ context.Context, _, _, parent, name, _, _ string, _ int64) (string, error) {
	f.files = append(f.files, parent+"/"+name)
	return "file-1", nil
}

func TestRecorderFinish(t *testing.T) {
	src := &memBlobs{data: map[string]string{"rec/1.mp4": "VIDEO"}}
	dst := &memBlobs{data: map[string]string{}}
	dr := &fakeDrive{}
	var notified, card, saved string
	rc := &Recorder{
		Src: src, Dst: dst, Drive: dr,
		Notify:   func(_ context.Context, _, user, title, _, url string) { notified = user + "|" + title + "|" + url },
		Channel:  func(context.Context, string, string) string { return "chan-1" },
		PostCard: func(_ context.Context, _, ch, _, _, body string) error { card = ch + "|" + body; return nil },
		Saved:    func(_ context.Context, _, status, id string) { saved = status + ":" + id },
	}
	err := rc.Finish(context.Background(), RecordingJob{EgressID: "EG", OrgID: "o", RoomID: "r", Code: "abc-defg-hij",
		RoomName: "Standup", StartedBy: "host", StarterNm: "Ann", Key: "rec/1.mp4", StartedAt: time.Date(2026, 11, 17, 10, 5, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if dst.data["blobs/aa/bb"] != "VIDEO" {
		t.Fatal("not copied to drive")
	}
	if _, still := src.data["rec/1.mp4"]; still {
		t.Fatal("source object must be removed")
	}
	if len(dr.files) != 1 || dr.files[0] != "folder-1/Standup - 2026-11-17 10-05.mp4" {
		t.Fatalf("files=%v", dr.files)
	}
	if notified != "host|Meeting recording saved|/drive/file/file-1" {
		t.Fatalf("notify=%q", notified)
	}
	if !strings.HasPrefix(card, "chan-1|") || !strings.Contains(card, "](/drive/file/file-1)") {
		t.Fatalf("card=%q", card)
	}
	if saved != "saved:file-1" {
		t.Fatalf("saved=%q", saved)
	}
}

func TestRecorderNoChannelNoCard(t *testing.T) {
	src := &memBlobs{data: map[string]string{"k": "x"}}
	called := false
	rc := &Recorder{Src: src, Dst: &memBlobs{data: map[string]string{}}, Drive: &fakeDrive{},
		Channel:  func(context.Context, string, string) string { return "" },
		PostCard: func(context.Context, string, string, string, string, string) error { called = true; return nil }}
	if err := rc.Finish(context.Background(), RecordingJob{Key: "k", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("no channel, no card")
	}
}

func TestRecorderFailureMarksFailed(t *testing.T) {
	var saved string
	rc := &Recorder{Src: &memBlobs{data: map[string]string{}}, Dst: &memBlobs{}, Drive: &fakeDrive{},
		Saved: func(_ context.Context, _, status, _ string) { saved = status }}
	if err := rc.Finish(context.Background(), RecordingJob{Key: "missing"}); err == nil || saved != "failed" {
		t.Fatalf("err=%v saved=%q", err, saved)
	}
}

// memStore is an in-memory StateStore standing in for Postgres.
type memStore struct {
	mu sync.Mutex
	m  map[string]persistedState
}

func (s *memStore) LoadState(_ context.Context, id string) (*persistedState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}
func (s *memStore) SaveState(_ context.Context, id string, p persistedState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = p
	return nil
}

const testLK = "org-o-11111111-2222-3333-4444-555555555555"

func TestStateSurvivesRestart(t *testing.T) {
	store := &memStore{m: map[string]persistedState{}}
	h, _ := newTestSFU()
	h.Store = store
	req := func(h *SFUHandler, body string) {
		r := httpReq(body)
		h.control(r.w, r.r, testLK, "11111111-2222-3333-4444-555555555555", "host")
	}
	req(h, `{"action":"lock"}`)
	req(h, `{"action":"waiting_room_on"}`)
	h.mu.Lock()
	h.state(testLK).banned["bad"] = true
	h.mu.Unlock()
	h.persist(testLK)

	h2, _ := newTestSFU() // "restarted backend": fresh memory, same store
	h2.Store = store
	h2.mu.Lock()
	st := h2.state(testLK)
	locked, wr, banned := st.locked, st.waitingRoom, st.banned["bad"]
	h2.mu.Unlock()
	if !locked || !wr || !banned {
		t.Fatalf("locked=%v wr=%v banned=%v", locked, wr, banned)
	}
}

func TestMustWait(t *testing.T) {
	h, _ := newTestSFU()
	st := newRoomState()
	ctx := context.Background()
	if h.mustWait(ctx, st, "u") {
		t.Fatal("no waiting room: nobody waits")
	}
	st.waitingRoom = true
	if !h.mustWait(ctx, st, "u") {
		t.Fatal("waiting room on, no user types yet: everyone waits")
	}
	st.admitted["u"] = true
	if h.mustWait(ctx, st, "u") {
		t.Fatal("admitted users do not wait again")
	}
	delete(st.admitted, "u")
	h.ExternalCheck = func(context.Context) bool { return false }
	if h.mustWait(ctx, st, "u") {
		t.Fatal("org members join directly")
	}
	h.ExternalCheck = func(context.Context) bool { return true }
	if !h.mustWait(ctx, st, "u") {
		t.Fatal("externals wait")
	}
}

func TestRecordingHostOnly(t *testing.T) {
	h, _ := newTestSFU()
	h.cfg.EgressEnabled = true
	r := httpReq(`{"action":"start"}`)
	h.recording(r.w, r.r, "org-o-r", "o", "r", "guest", "Gus", Room{}, false)
	if r.w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", r.w.Code)
	}
	r = httpReq(`{"action":"start"}`)
	h.recording(r.w, r.r, "org-o-r", "o", "r", "host", "Ann", Room{Name: "x"}, true)
	if r.w.Code != 200 {
		t.Fatalf("host start code=%d", r.w.Code)
	}
}
