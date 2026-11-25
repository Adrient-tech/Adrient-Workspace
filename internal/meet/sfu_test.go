package meet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
)

type fakeAdmin struct {
	muted, removed []string
	deleted        []string
	mutedAll       int
}

func (f *fakeAdmin) MuteAudio(_ context.Context, _, id string) error {
	f.muted = append(f.muted, id)
	return nil
}
func (f *fakeAdmin) MuteAllAudio(_ context.Context, _ string, except func(string) bool) (int, error) {
	if !except("host") || except("guest") {
		panic("except func must spare only the host")
	}
	f.mutedAll++
	return 3, nil
}
func (f *fakeAdmin) Remove(_ context.Context, _, id string) error {
	f.removed = append(f.removed, id)
	return nil
}
func (f *fakeAdmin) Delete(_ context.Context, r string) error {
	f.deleted = append(f.deleted, r)
	return nil
}
func (f *fakeAdmin) StartRecording(context.Context, string, string) (string, error) { return "EG_1", nil }
func (f *fakeAdmin) StopRecording(context.Context, string) error                      { return nil }

func (f *fakeAdmin) EgressStatus(context.Context, string) (bool, bool, error) { return true, true, nil }

func newTestSFU() (*SFUHandler, *fakeAdmin) {
	fa := &fakeAdmin{}
	h := &SFUHandler{
		cfg:    SFUConfig{APIURL: "http://x", ClientURL: "ws://x", APIKey: "devkey", Secret: "0123456789abcdef0123456789abcdef"},
		admin:  fa,
		states: map[string]*roomState{},
		now:    time.Now,
	}
	return h, fa
}

func join(t *testing.T, h *SFUHandler, uid, cid string, host bool) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"conn_id":"`+cid+`","name":"Ann"}`))
	w := httptest.NewRecorder()
	h.joinToken(w, req, "org-o-r", "r", "host", uid, "fallback", host, false)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func ctl(t *testing.T, h *SFUHandler, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.control(w, req, "org-o-r", "r", "host")
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestJoinTokenClaims(t *testing.T) {
	h, _ := newTestSFU()
	code, out := join(t, h, "u1", "abc", false)
	if code != 200 || out["identity"] != "u1~abc" || out["url"] != "ws://x" {
		t.Fatalf("got %d %v", code, out)
	}
	v, err := auth.ParseAPIToken(out["token"].(string))
	if err != nil {
		t.Fatal(err)
	}
	_, claims, err := v.Verify(h.cfg.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Video.Room != "org-o-r" || !claims.Video.RoomJoin || claims.Identity != "u1~abc" || claims.Name != "Ann" {
		t.Fatalf("claims %+v", claims)
	}
	if claims.Video.RoomAdmin {
		t.Fatal("participants must not get admin")
	}
}

func TestWaitingRoomAdmitDeny(t *testing.T) {
	h, _ := newTestSFU()
	ctl(t, h, `{"action":"waiting_room_on"}`)
	code, out := join(t, h, "g1", "c1", false)
	if code != http.StatusAccepted || out["status"] != "waiting" || out["token"] != nil {
		t.Fatalf("no token before admit: %d %v", code, out)
	}
	// host can always join
	if c, o := join(t, h, "host", "h", true); c != 200 || o["token"] == nil {
		t.Fatalf("host: %d %v", c, o)
	}
	if n := len(h.state("org-o-r").waiting); n != 1 {
		t.Fatalf("waiting=%d", n)
	}
	if c, _ := ctl(t, h, `{"action":"admit","identity":"g1~c1"}`); c != 200 {
		t.Fatal("admit", c)
	}
	if c, o := join(t, h, "g1", "c1", false); c != 200 || o["token"] == nil {
		t.Fatalf("after admit: %d %v", c, o)
	}
	// an admitted user can refresh and rejoin without waiting again
	if c, _ := join(t, h, "g1", "c2", false); c != 200 {
		t.Fatalf("admitted user must rejoin, got %d", c)
	}
	join(t, h, "g2", "c1", false)
	ctl(t, h, `{"action":"deny","identity":"g2~c1"}`)
	if c, o := join(t, h, "g2", "c1", false); c != 403 || o["error"] != "denied" {
		t.Fatalf("deny: %d %v", c, o)
	}
}

func TestLockRemoveBan(t *testing.T) {
	h, fa := newTestSFU()
	ctl(t, h, `{"action":"lock"}`)
	if c, o := join(t, h, "u2", "x", false); c != 403 || o["error"] != "room_locked" {
		t.Fatalf("%d %v", c, o)
	}
	if c, _ := join(t, h, "host", "x", true); c != 200 {
		t.Fatal("host bypasses lock")
	}
	ctl(t, h, `{"action":"unlock"}`)
	if c, _ := join(t, h, "u2", "x", false); c != 200 {
		t.Fatal("unlocked join")
	}
	if c, _ := ctl(t, h, `{"action":"remove","identity":"u2~x"}`); c != 200 || len(fa.removed) != 1 {
		t.Fatal("remove")
	}
	if c, o := join(t, h, "u2", "y", false); c != 403 || o["error"] != "removed_by_host" {
		t.Fatalf("banned user can rejoin: %d %v", c, o)
	}
	if c, _ := ctl(t, h, `{"action":"remove","identity":"host~1"}`); c != 400 {
		t.Fatal("host cannot be removed")
	}
}

func TestMuteAndEnd(t *testing.T) {
	h, fa := newTestSFU()
	ctl(t, h, `{"action":"mute","identity":"u3~a"}`)
	if len(fa.muted) != 1 || fa.muted[0] != "u3~a" {
		t.Fatal("mute")
	}
	if c, o := ctl(t, h, `{"action":"mute_all"}`); c != 200 || o["muted"] != float64(3) {
		t.Fatalf("%d %v", c, o)
	}
	ctl(t, h, `{"action":"lock"}`)
	ctl(t, h, `{"action":"end"}`)
	if len(fa.deleted) != 1 || len(h.states) != 0 {
		t.Fatal("end must delete the room and clear state")
	}
	if c, _ := ctl(t, h, `{"action":"nope"}`); c != 400 {
		t.Fatal("unknown action")
	}
}

func TestWaiterExpiry(t *testing.T) {
	h, _ := newTestSFU()
	ctl(t, h, `{"action":"waiting_room_on"}`)
	join(t, h, "g", "c", false)
	h.now = func() time.Time { return time.Now().Add(time.Minute) }
	w := httptest.NewRecorder()
	h.stateJSON(context.Background(), w, "org-o-r", true)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if l := out["waiting"].([]any); len(l) != 0 {
		t.Fatalf("stale waiter kept: %v", l)
	}
}

func TestMatchAndDisabled(t *testing.T) {
	h, _ := newTestSFU()
	if id, a, ok := h.Match("/api/v1/meet/rooms/abc/join-token"); !ok || id != "abc" || a != "join-token" {
		t.Fatal("match")
	}
	for _, p := range []string{"/api/v1/meet/rooms/abc", "/api/v1/meet/rooms/abc/connect", "/api/v1/meet/rooms//control"} {
		if _, _, ok := h.Match(p); ok {
			t.Fatalf("must not match %s", p)
		}
	}
	h.cfg.APIKey = ""
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/meet/rooms/abc/join-token", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("disabled sfu must 503")
	}
}

func TestHubIgnoresForgedServerMessages(t *testing.T) {
	h := NewHub()
	cr := &callRoom{peers: map[string]*peer{}}
	a := &peer{id: "a", out: make(chan []byte, 4)}
	b := &peer{id: "b", out: make(chan []byte, 4)}
	cr.peers["a"], cr.peers["b"] = a, b
	h.route(cr, a, SignalMessage{Type: SignalLobby, From: "a"})
	h.route(cr, a, SignalMessage{Type: SignalRoomState, From: "a"})
	if len(b.out) != 0 {
		t.Fatal("client-sent lobby/room_state must not be relayed")
	}
}
