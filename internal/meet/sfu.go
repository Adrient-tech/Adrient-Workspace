package meet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
)

// SFUConfig configures the self-hosted LiveKit SFU. Empty APIKey/Secret/URL
// disables SFU calls (join-token answers 503).
type SFUConfig struct {
	// APIURL is the server-side LiveKit URL, e.g. http://127.0.0.1:7880.
	APIURL string
	// ClientURL is the URL browsers connect to, e.g. wss://rtc.example.com.
	ClientURL string
	APIKey    string
	Secret    string
	// Egress (recording) is a separate feature flag: it needs the egress
	// container, which not every install runs.
	EgressEnabled  bool
	S3Endpoint     string // as egress reaches RustFS
	S3ReadEndpoint string // as the backend reaches RustFS (defaults to S3Endpoint)
	S3Bucket       string
	S3Region       string
	S3AccessKey    string
	S3Secret       string
}

// SFUConfigFromEnv reads ADRIENT_LIVEKIT_* variables.
func SFUConfigFromEnv() SFUConfig {
	api := os.Getenv("ADRIENT_LIVEKIT_URL")
	client := os.Getenv("ADRIENT_LIVEKIT_CLIENT_URL")
	if client == "" {
		client = api
	}
	return SFUConfig{
		APIURL:         api,
		ClientURL:      toWS(client),
		APIKey:         os.Getenv("ADRIENT_LIVEKIT_API_KEY"),
		Secret:         os.Getenv("ADRIENT_LIVEKIT_API_SECRET"),
		EgressEnabled:  os.Getenv("ADRIENT_LIVEKIT_EGRESS") == "1",
		S3Endpoint:     os.Getenv("ADRIENT_LIVEKIT_EGRESS_S3_ENDPOINT"),
		S3ReadEndpoint: envOr("ADRIENT_LIVEKIT_EGRESS_S3_READ_ENDPOINT", os.Getenv("ADRIENT_LIVEKIT_EGRESS_S3_ENDPOINT")),
		S3Bucket:       os.Getenv("ADRIENT_LIVEKIT_EGRESS_S3_BUCKET"),
		S3Region:       envOr("ADRIENT_LIVEKIT_EGRESS_S3_REGION", "us-east-1"),
		S3AccessKey:    os.Getenv("ADRIENT_LIVEKIT_EGRESS_S3_ACCESS_KEY"),
		S3Secret:       os.Getenv("ADRIENT_LIVEKIT_EGRESS_S3_SECRET_KEY"),
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func toWS(u string) string {
	switch {
	case strings.HasPrefix(u, "http://"):
		return "ws://" + strings.TrimPrefix(u, "http://")
	case strings.HasPrefix(u, "https://"):
		return "wss://" + strings.TrimPrefix(u, "https://")
	}
	return u
}

// Enabled reports whether the SFU is configured.
func (c SFUConfig) Enabled() bool { return c.APIURL != "" && c.APIKey != "" && c.Secret != "" }

// SFUAdmin is the subset of LiveKit server APIs the host controls need.
type SFUAdmin interface {
	MuteAudio(ctx context.Context, room, identity string) error
	MuteAllAudio(ctx context.Context, room string, except func(identity string) bool) (int, error)
	Remove(ctx context.Context, room, identity string) error
	Delete(ctx context.Context, room string) error
	StartRecording(ctx context.Context, room, path string) (string, error)
	StopRecording(ctx context.Context, egressID string) error
	// EgressStatus reports whether the egress finished ("done") and whether it
	// produced a file ("ok"). Not finished: done=false.
	EgressStatus(ctx context.Context, egressID string) (done, ok bool, err error)
}

type lkAdmin struct {
	cfg  SFUConfig
	room livekit.RoomService
	egr  livekit.Egress
}

func newLKAdmin(cfg SFUConfig) *lkAdmin {
	hc := &http.Client{Timeout: 10 * time.Second}
	return &lkAdmin{
		cfg:  cfg,
		room: livekit.NewRoomServiceProtobufClient(cfg.APIURL, hc),
		egr:  livekit.NewEgressProtobufClient(cfg.APIURL, hc),
	}
}

func (a *lkAdmin) ctx(ctx context.Context, grant *auth.VideoGrant) (context.Context, error) {
	jwt, err := auth.NewAccessToken(a.cfg.APIKey, a.cfg.Secret).SetVideoGrant(grant).SetValidFor(time.Minute).ToJWT()
	if err != nil {
		return ctx, err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+jwt)
	return twirp.WithHTTPRequestHeaders(ctx, h)
}

func (a *lkAdmin) adminCtx(ctx context.Context, room string) (context.Context, error) {
	return a.ctx(ctx, &auth.VideoGrant{RoomAdmin: true, Room: room, RoomCreate: true, RoomList: true})
}

func (a *lkAdmin) MuteAudio(ctx context.Context, room, identity string) error {
	c, err := a.adminCtx(ctx, room)
	if err != nil {
		return err
	}
	p, err := a.room.GetParticipant(c, &livekit.RoomParticipantIdentity{Room: room, Identity: identity})
	if err != nil {
		return err
	}
	for _, t := range p.Tracks {
		if t.Type == livekit.TrackType_AUDIO && !t.Muted {
			if _, err := a.room.MutePublishedTrack(c, &livekit.MuteRoomTrackRequest{Room: room, Identity: identity, TrackSid: t.Sid, Muted: true}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *lkAdmin) MuteAllAudio(ctx context.Context, room string, except func(string) bool) (int, error) {
	c, err := a.adminCtx(ctx, room)
	if err != nil {
		return 0, err
	}
	res, err := a.room.ListParticipants(c, &livekit.ListParticipantsRequest{Room: room})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range res.Participants {
		if except != nil && except(p.Identity) {
			continue
		}
		for _, t := range p.Tracks {
			if t.Type == livekit.TrackType_AUDIO && !t.Muted {
				if _, err := a.room.MutePublishedTrack(c, &livekit.MuteRoomTrackRequest{Room: room, Identity: p.Identity, TrackSid: t.Sid, Muted: true}); err == nil {
					n++
				}
			}
		}
	}
	return n, nil
}

func (a *lkAdmin) Remove(ctx context.Context, room, identity string) error {
	c, err := a.adminCtx(ctx, room)
	if err != nil {
		return err
	}
	_, err = a.room.RemoveParticipant(c, &livekit.RoomParticipantIdentity{Room: room, Identity: identity})
	return err
}

func (a *lkAdmin) Delete(ctx context.Context, room string) error {
	c, err := a.adminCtx(ctx, room)
	if err != nil {
		return err
	}
	_, err = a.room.DeleteRoom(c, &livekit.DeleteRoomRequest{Room: room})
	return err
}

func (a *lkAdmin) StartRecording(ctx context.Context, room, path string) (string, error) {
	c, err := a.ctx(ctx, &auth.VideoGrant{RoomRecord: true})
	if err != nil {
		return "", err
	}
	info, err := a.egr.StartRoomCompositeEgress(c, &livekit.RoomCompositeEgressRequest{
		RoomName: room,
		Layout:   "grid",
		Output: &livekit.RoomCompositeEgressRequest_File{File: &livekit.EncodedFileOutput{
			FileType: livekit.EncodedFileType_MP4,
			Filepath: path,
			Output: &livekit.EncodedFileOutput_S3{S3: &livekit.S3Upload{
				AccessKey:      a.cfg.S3AccessKey,
				Secret:         a.cfg.S3Secret,
				Region:         a.cfg.S3Region,
				Endpoint:       a.cfg.S3Endpoint,
				Bucket:         a.cfg.S3Bucket,
				ForcePathStyle: true,
			}},
		}},
	})
	if err != nil {
		return "", err
	}
	return info.EgressId, nil
}

func (a *lkAdmin) StopRecording(ctx context.Context, egressID string) error {
	c, err := a.ctx(ctx, &auth.VideoGrant{RoomRecord: true})
	if err != nil {
		return err
	}
	_, err = a.egr.StopEgress(c, &livekit.StopEgressRequest{EgressId: egressID})
	return err
}

func (a *lkAdmin) EgressStatus(ctx context.Context, egressID string) (bool, bool, error) {
	c, err := a.ctx(ctx, &auth.VideoGrant{RoomRecord: true})
	if err != nil {
		return false, false, err
	}
	res, err := a.egr.ListEgress(c, &livekit.ListEgressRequest{EgressId: egressID})
	if err != nil {
		return false, false, err
	}
	if len(res.Items) == 0 {
		return true, false, nil
	}
	switch res.Items[0].Status {
	case livekit.EgressStatus_EGRESS_COMPLETE:
		return true, true, nil
	case livekit.EgressStatus_EGRESS_FAILED, livekit.EgressStatus_EGRESS_ABORTED, livekit.EgressStatus_EGRESS_LIMIT_REACHED:
		return true, false, nil
	}
	return false, false, nil
}

// waiter is one person in the waiting room.
type waiter struct {
	Identity string
	Name     string
	Since    time.Time
	LastSeen time.Time
}

// roomState is the ephemeral per-call state (lock, waiting room, bans).
type roomState struct {
	locked       bool
	waitingRoom  bool
	admitted     map[string]bool // user id -> admitted for this call (survives a refresh)
	denied       map[string]bool
	banned       map[string]bool // user id
	waiting      map[string]*waiter
	egressID     string
	hostID       string // current host user id (defaults to the room owner)
	transcribing bool   // host switched the transcript on (memory only: a restart turns it off)
	loaded       bool
}

func newRoomState() *roomState {
	return &roomState{
		admitted: map[string]bool{}, denied: map[string]bool{},
		banned: map[string]bool{}, waiting: map[string]*waiter{},
	}
}

// waiterTTL: a waiting joiner polls every ~2s; silence this long means they left.
const waiterTTL = 20 * time.Second

// SFUHandler serves /api/v1/meet/rooms/{id}/{join-token,sfu-state,control,recording}.
type SFUHandler struct {
	cfg   SFUConfig
	repo  *Repository
	hub   *Hub
	admin SFUAdmin
	// CanOpen reports app access; nil means allowed.
	CanOpen func(ctx context.Context, app string) bool

	orgFromCtx  func(r *http.Request) (string, bool)
	userFromCtx func(r *http.Request) (id, name string, ok bool)

	// Store persists lock/waiting-room/ban/host state (nil = memory only).
	Store StateStore
	// Rec finishes recordings into Drive (nil = recordings stay in the egress bucket).
	Rec *Recorder
	// RecDB tracks recordings across restarts (nil = not tracked).
	RecDB RecordingDB
	// ExternalCheck reports whether the caller is a guest/external user. Until the
	// identity agent ships user types it is nil, and mustWait keeps the current
	// behaviour (everybody waits when the waiting room is on). The wiring in
	// server.go sets it from authz.IsExternal once that exists.
	ExternalCheck func(ctx context.Context) bool
	// Control reads a per-app control for the caller ("meet.recording" returns a
	// string, "meet.lobby_for_external" a bool). nil = defaults (host records,
	// externals wait only when the host turned the waiting room on).
	Control func(ctx context.Context, key string) any
	// JoinCheck refuses a join before any token is issued (guests not allowed,
	// blocked partner domain). nil = allow.
	JoinCheck func(ctx context.Context) error
	// AnonAllowed decides meet.allow_anonymous for an organization (resolved for
	// the "everyone" subject). nil = anonymous join is off.
	AnonAllowed func(ctx context.Context, orgID string) bool
	// OnEnd runs after the host ended the meeting for everyone.
	OnEnd func(roomID, reason string)

	mu       sync.Mutex
	anonHits map[string][]time.Time
	states   map[string]*roomState // lk room name -> state
	now      func() time.Time
	recWait  time.Duration
	recPoll  time.Duration
}

// RecordingDB is the durable record of recordings.
type RecordingDB interface {
	StartRecording(ctx context.Context, job RecordingJob) error
	FinishRecording(ctx context.Context, egressID, status, driveFileID string) error
	PendingRecordings(ctx context.Context) ([]RecordingJob, error)
}

// NewSFUHandler builds the handler. userFromCtx returns the caller's id and display name.
func NewSFUHandler(cfg SFUConfig, repo *Repository, hub *Hub,
	orgFromCtx func(r *http.Request) (string, bool),
	userFromCtx func(r *http.Request) (string, string, bool)) *SFUHandler {
	h := &SFUHandler{cfg: cfg, repo: repo, hub: hub, orgFromCtx: orgFromCtx, userFromCtx: userFromCtx,
		states: map[string]*roomState{}, now: time.Now,
		recWait: 4 * time.Hour, recPoll: 3 * time.Second}
	if repo != nil {
		h.Store = repo
		h.RecDB = repo
	}
	if cfg.Enabled() {
		h.admin = newLKAdmin(cfg)
	}
	return h
}

// LKRoomName is the org-scoped LiveKit room name.
func LKRoomName(orgID, roomID string) string { return "org-" + orgID + "-" + roomID }

const roomsMount = "/api/v1/meet/rooms/"

var sfuActions = map[string]bool{"join-token": true, "sfu-state": true, "control": true, "recording": true, "transcript": true}

// Match returns the room id and action for a routed path.
func (h *SFUHandler) Match(path string) (roomID, action string, ok bool) {
	if !strings.HasPrefix(path, roomsMount) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, roomsMount), "/")
	if len(parts) != 2 || parts[0] == "" || !sfuActions[parts[1]] {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// roomIDOf extracts the meet room id (a 36-char uuid) from an LK room name.
func roomIDOf(name string) string {
	if len(name) < 36 {
		return ""
	}
	return name[len(name)-36:]
}

// state returns the room state, loading it from the store on first use.
// Callers hold h.mu.
func (h *SFUHandler) state(name string) *roomState {
	s, ok := h.states[name]
	if !ok {
		s = newRoomState()
		h.states[name] = s
		if h.Store != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			p, err := h.Store.LoadState(ctx, roomIDOf(name))
			cancel()
			if err == nil && p != nil {
				s.hostID, s.locked, s.waitingRoom, s.egressID = p.HostID, p.Locked, p.WaitingRoom, p.EgressID
				for _, u := range p.Banned {
					s.banned[u] = true
				}
				for _, u := range p.Admitted {
					s.admitted[u] = true
				}
				for _, u := range p.Denied {
					s.denied[u] = true
				}
				for i := range p.Waiting {
					w := p.Waiting[i]
					s.waiting[w.Identity] = &w
				}
			}
		}
	}
	return s
}

// persist writes the room state through to the store. Call WITHOUT h.mu held.
func (h *SFUHandler) persist(name string) {
	if h.Store == nil {
		return
	}
	h.mu.Lock()
	st := h.state(name)
	p := persistedState{HostID: st.hostID, Locked: st.locked, WaitingRoom: st.waitingRoom, EgressID: st.egressID}
	for u := range st.banned {
		p.Banned = append(p.Banned, u)
	}
	for u := range st.admitted {
		p.Admitted = append(p.Admitted, u)
	}
	for u := range st.denied {
		p.Denied = append(p.Denied, u)
	}
	for _, w := range st.waiting {
		p.Waiting = append(p.Waiting, *w)
	}
	h.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Store.SaveState(ctx, roomIDOf(name), p); err != nil {
		slog.Warn("meet: persist room state", "room", name, "err", err)
	}
}

// mustWait decides whether a (non-host) joiner has to wait in the lobby.
// Until user types exist everybody waits when the waiting room is on; once
// ExternalCheck is wired only guests/externals wait and org members join
// directly. A locked room is handled separately (nobody but the host joins).
// Callers hold h.mu.
func (h *SFUHandler) mustWait(ctx context.Context, st *roomState, uid string) bool {
	if st.admitted[uid] {
		return false
	}
	// External participants always wait for the host when meet.lobby_for_external
	// is on, even if the host did not switch the waiting room on.
	if h.ExternalCheck != nil && h.ExternalCheck(ctx) && h.ctlBool(ctx, "meet.lobby_for_external", false) {
		return true
	}
	if !st.waitingRoom {
		return false
	}
	if h.ExternalCheck != nil && !h.ExternalCheck(ctx) {
		return false
	}
	return true
}

func (h *SFUHandler) ctlBool(ctx context.Context, key string, def bool) bool {
	if h.Control == nil {
		return def
	}
	if b, ok := h.Control(ctx, key).(bool); ok {
		return b
	}
	return def
}

func (h *SFUHandler) ctlString(ctx context.Context, key, def string) string {
	if h.Control == nil {
		return def
	}
	if v, ok := h.Control(ctx, key).(string); ok && v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (h *SFUHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	roomID, action, ok := h.Match(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !h.cfg.Enabled() {
		jerr(w, http.StatusServiceUnavailable, "sfu_not_configured")
		return
	}
	orgID, ok1 := h.orgFromCtx(r)
	uid, uname, ok2 := h.userFromCtx(r)
	if !ok1 || !ok2 {
		jerr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.CanOpen != nil && !h.CanOpen(r.Context(), "meet") {
		jerr(w, http.StatusForbidden, "app_access_denied")
		return
	}
	room, err := h.repo.Get(r.Context(), orgID, roomID)
	if err != nil {
		jerr(w, http.StatusNotFound, "room_not_found")
		return
	}
	name := LKRoomName(orgID, roomID)
	h.mu.Lock()
	hostUser := h.state(name).hostID
	h.mu.Unlock()
	if hostUser == "" {
		hostUser = room.OwnerID
	}
	isHost := room.OwnerID == uid || hostUser == uid
	switch action {
	case "join-token":
		if r.Method != http.MethodPost {
			jerr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		if room.Ended() {
			jerr(w, http.StatusGone, "meeting_ended")
			return
		}
		if h.JoinCheck != nil && !isHost {
			if jerrC := h.JoinCheck(r.Context()); jerrC != nil {
				jerr(w, http.StatusForbidden, jerrC.Error())
				return
			}
		}
		h.joinToken(w, r, name, roomID, hostUser, uid, uname, isHost, false)
	case "sfu-state":
		h.stateJSON(r.Context(), w, name, isHost)
	case "control":
		if r.Method != http.MethodPost {
			jerr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		if !isHost {
			jerr(w, http.StatusForbidden, "host_only")
			return
		}
		h.control(w, r, name, roomID, hostUser)
	case "transcript":
		h.transcript(w, r, name, orgID, roomID, uid, uname, room, isHost)
	case "recording":
		if r.Method != http.MethodPost {
			jerr(w, http.StatusMethodNotAllowed, "method")
			return
		}
		switch h.ctlString(r.Context(), "meet.recording", "host") {
		case "off":
			jerr(w, http.StatusForbidden, "recording_off")
			return
		case "all":
			// any participant may record
		default:
			if !isHost {
				jerr(w, http.StatusForbidden, "host_only")
				return
			}
		}
		h.recording(w, r, name, orgID, roomID, uid, uname, room, true)
	}
}

type joinReq struct {
	ConnID string `json:"conn_id"`
	Name   string `json:"name"`
}

func (h *SFUHandler) joinToken(w http.ResponseWriter, r *http.Request, name, roomID, hostID, uid, uname string, isHost, forceWait bool) {
	var req joinReq
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	identity := PeerID(uid, req.ConnID)
	display := CleanDisplayName(req.Name, uname)

	h.mu.Lock()
	st := h.state(name)
	if !isHost {
		if st.banned[uid] {
			h.mu.Unlock()
			jerr(w, http.StatusForbidden, "removed_by_host")
			return
		}
		if st.locked {
			h.mu.Unlock()
			jerr(w, http.StatusForbidden, "room_locked")
			return
		}
		if h.mustWait(r.Context(), st, uid) || (forceWait && !st.admitted[uid]) {
			if st.denied[identity] {
				h.mu.Unlock()
				jerr(w, http.StatusForbidden, "denied")
				return
			}
			now := h.now()
			wt, exists := st.waiting[identity]
			if !exists {
				wt = &waiter{Identity: identity, Name: display, Since: now}
				st.waiting[identity] = wt
			}
			wt.LastSeen = now
			h.mu.Unlock()
			if !exists {
				h.persist(name)
				h.notifyLobby(name, roomID, hostID)
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "waiting"})
			return
		}
		_, wasWaiting := st.waiting[identity]
		delete(st.waiting, identity)
		if wasWaiting {
			h.mu.Unlock()
			h.persist(name)
			h.mu.Lock()
		}
	}
	h.mu.Unlock()

	meta, _ := json.Marshal(map[string]any{"host": isHost, "anonymous": forceWait})
	t := true
	grant := &auth.VideoGrant{RoomJoin: true, Room: name, CanPublish: &t, CanSubscribe: &t, CanPublishData: &t}
	jwt, err := auth.NewAccessToken(h.cfg.APIKey, h.cfg.Secret).
		SetVideoGrant(grant).SetIdentity(identity).SetName(display).
		SetMetadata(string(meta)).SetValidFor(6 * time.Hour).ToJWT()
	if err != nil {
		jerr(w, http.StatusInternalServerError, "sign_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "token": jwt, "url": h.cfg.ClientURL, "identity": identity,
		"name": display, "is_host": isHost, "room": name,
	})
}

func (h *SFUHandler) stateJSON(ctx context.Context, w http.ResponseWriter, name string, isHost bool) {
	mode := h.ctlString(ctx, "meet.recording", "host")
	canRecord := h.cfg.EgressEnabled && mode != "off" && (isHost || mode == "all")
	h.mu.Lock()
	st := h.state(name)
	h.pruneLocked(st)
	out := map[string]any{
		"locked": st.locked, "waiting_room": st.waitingRoom, "is_host": isHost,
		"recording_enabled": h.cfg.EgressEnabled, "recording": st.egressID != "", "can_record": canRecord,
		"waiting": []map[string]any{}, "transcribing": st.transcribing,
	}
	if isHost {
		out["waiting"] = h.waitersLocked(st)
	}
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (h *SFUHandler) pruneLocked(st *roomState) {
	cut := h.now().Add(-waiterTTL)
	for id, wt := range st.waiting {
		if wt.LastSeen.Before(cut) {
			delete(st.waiting, id)
		}
	}
}

func (h *SFUHandler) waitersLocked(st *roomState) []map[string]any {
	ws := make([]*waiter, 0, len(st.waiting))
	for _, wt := range st.waiting {
		ws = append(ws, wt)
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].Since.Before(ws[j].Since) })
	out := make([]map[string]any, 0, len(ws))
	for _, wt := range ws {
		out = append(out, map[string]any{"identity": wt.Identity, "name": wt.Name})
	}
	return out
}

// notifyLobby tells the host's open call tabs that the waiting list changed.
func (h *SFUHandler) notifyLobby(name, roomID, hostID string) {
	if h.hub == nil {
		return
	}
	h.mu.Lock()
	st := h.state(name)
	h.pruneLocked(st)
	list := h.waitersLocked(st)
	h.mu.Unlock()
	payload, _ := json.Marshal(map[string]any{"waiting": list})
	h.hub.Send(roomID, func(peerID string) bool { return UserOfPeer(peerID) == hostID },
		SignalMessage{Type: SignalLobby, Payload: payload})
}

type controlReq struct {
	Action   string `json:"action"`
	Identity string `json:"identity"`
}

func (h *SFUHandler) control(w http.ResponseWriter, r *http.Request, name, roomID, hostID string) {
	var req controlReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		jerr(w, http.StatusBadRequest, "bad_request")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	isHostID := func(identity string) bool { return UserOfPeer(identity) == hostID }
	fail := func(err error) {
		var te twirp.Error
		if errors.As(err, &te) && te.Code() == twirp.NotFound {
			jerr(w, http.StatusNotFound, "not_found")
			return
		}
		jerr(w, http.StatusBadGateway, "sfu_error")
	}
	switch req.Action {
	case "mute":
		if req.Identity == "" {
			jerr(w, http.StatusBadRequest, "identity_required")
			return
		}
		if err := h.admin.MuteAudio(ctx, name, req.Identity); err != nil {
			fail(err)
			return
		}
	case "mute_all":
		n, err := h.admin.MuteAllAudio(ctx, name, isHostID)
		if err != nil {
			fail(err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "muted": n})
		return
	case "remove":
		if req.Identity == "" || isHostID(req.Identity) {
			jerr(w, http.StatusBadRequest, "identity_invalid")
			return
		}
		h.mu.Lock()
		h.state(name).banned[UserOfPeer(req.Identity)] = true
		h.mu.Unlock()
		h.persist(name)
		if err := h.admin.Remove(ctx, name, req.Identity); err != nil {
			fail(err)
			return
		}
	case "lock", "unlock":
		h.mu.Lock()
		h.state(name).locked = req.Action == "lock"
		h.mu.Unlock()
		h.persist(name)
	case "waiting_room_on", "waiting_room_off":
		h.mu.Lock()
		st := h.state(name)
		st.waitingRoom = req.Action == "waiting_room_on"
		h.mu.Unlock()
		h.persist(name)
	case "admit", "deny":
		h.mu.Lock()
		st := h.state(name)
		if _, ok := st.waiting[req.Identity]; !ok {
			h.mu.Unlock()
			jerr(w, http.StatusNotFound, "not_waiting")
			return
		}
		delete(st.waiting, req.Identity)
		if req.Action == "admit" {
			st.admitted[UserOfPeer(req.Identity)] = true
		} else {
			st.denied[req.Identity] = true
		}
		h.mu.Unlock()
		h.persist(name)
		h.notifyLobby(name, roomID, hostID)
	case "transcript_on", "transcript_off":
		h.mu.Lock()
		h.state(name).transcribing = req.Action == "transcript_on"
		h.mu.Unlock()
	case "make_host":
		if req.Identity == "" {
			jerr(w, http.StatusBadRequest, "identity_required")
			return
		}
		h.mu.Lock()
		h.state(name).hostID = UserOfPeer(req.Identity)
		h.mu.Unlock()
		h.persist(name)
	case "end":
		h.mu.Lock()
		eg := h.state(name).egressID
		delete(h.states, name)
		h.mu.Unlock()
		if h.Store != nil {
			if d, ok := h.Store.(interface {
				DeleteState(ctx context.Context, roomID string) error
			}); ok {
				_ = d.DeleteState(ctx, roomIDOf(name))
			}
		}
		if eg != "" {
			_ = h.admin.StopRecording(ctx, eg)
		}
		if h.hub != nil {
			h.hub.Send(roomID, nil, SignalMessage{Type: SignalMeetingEnded})
		}
		if h.OnEnd != nil {
			h.OnEnd(roomID, EndedHost)
		}
		if err := h.admin.Delete(ctx, name); err != nil {
			fail(err)
			return
		}
	default:
		jerr(w, http.StatusBadRequest, "unknown_action")
		return
	}
	if req.Action == "lock" || req.Action == "unlock" || req.Action == "waiting_room_on" || req.Action == "waiting_room_off" || req.Action == "transcript_on" || req.Action == "transcript_off" {
		h.broadcastState(name, roomID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// broadcastState tells every participant the lock/waiting-room flags changed.
func (h *SFUHandler) broadcastState(name, roomID string) {
	if h.hub == nil {
		return
	}
	h.mu.Lock()
	st := h.state(name)
	p, _ := json.Marshal(map[string]any{"locked": st.locked, "waiting_room": st.waitingRoom, "recording": st.egressID != "", "transcribing": st.transcribing})
	h.mu.Unlock()
	h.hub.Send(roomID, nil, SignalMessage{Type: SignalRoomState, Payload: p})
}

func (h *SFUHandler) recording(w http.ResponseWriter, r *http.Request, name, orgID, roomID, uid, uname string, room Room, canRecord bool) {
	if !h.cfg.EgressEnabled {
		jerr(w, http.StatusNotImplemented, "recording_disabled")
		return
	}
	// Only the host (room admin) records. Per-app controls will refine this.
	if !canRecord {
		jerr(w, http.StatusForbidden, "host_only")
		return
	}
	var req controlReq
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	h.mu.Lock()
	st := h.state(name)
	cur := st.egressID
	h.mu.Unlock()
	switch req.Action {
	case "start":
		if cur != "" {
			jerr(w, http.StatusConflict, "already_recording")
			return
		}
		var b [4]byte
		_, _ = rand.Read(b[:])
		path := fmt.Sprintf("meet-recordings/%s/%s/%s-%s.mp4", url.PathEscape(orgID), url.PathEscape(roomID),
			h.now().UTC().Format("20060102-150405"), hex.EncodeToString(b[:]))
		id, err := h.admin.StartRecording(ctx, name, path)
		if err != nil {
			jerr(w, http.StatusBadGateway, "egress_error")
			return
		}
		job := RecordingJob{EgressID: id, OrgID: orgID, RoomID: roomID, Code: room.Code, RoomName: room.Name,
			StartedBy: uid, StarterNm: uname, Key: path, StartedAt: h.now()}
		h.mu.Lock()
		h.state(name).egressID = id
		h.mu.Unlock()
		h.persist(name)
		if h.RecDB != nil {
			if err := h.RecDB.StartRecording(ctx, job); err != nil {
				slog.Warn("meet: record recording", "err", err)
			}
		}
		h.watchEgress(name, job)
		h.broadcastState(name, roomID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
	case "stop":
		if cur == "" {
			jerr(w, http.StatusConflict, "not_recording")
			return
		}
		if err := h.admin.StopRecording(ctx, cur); err != nil {
			jerr(w, http.StatusBadGateway, "egress_error")
			return
		}
		// The watcher clears the flag once the file is finalized; flip it now
		// for the UI.
		h.clearEgress(name, roomID, cur)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		jerr(w, http.StatusBadRequest, "unknown_action")
	}
}

// clearEgress marks the room as no longer recording (if egressID is still current).
func (h *SFUHandler) clearEgress(name, roomID, egressID string) {
	h.mu.Lock()
	st := h.state(name)
	changed := st.egressID == egressID
	if changed {
		st.egressID = ""
	}
	h.mu.Unlock()
	if changed {
		h.persist(name)
		h.broadcastState(name, roomID)
	}
}

// ResumeRecordings restarts the finalizers of recordings that were still
// running when the backend went down (egress keeps recording meanwhile).
func (h *SFUHandler) ResumeRecordings(ctx context.Context) {
	if h.RecDB == nil || h.Rec == nil || h.admin == nil {
		return
	}
	jobs, err := h.RecDB.PendingRecordings(ctx)
	if err != nil {
		slog.Warn("meet: pending recordings", "err", err)
		return
	}
	for _, j := range jobs {
		h.watchEgress(LKRoomName(j.OrgID, j.RoomID), j)
	}
}

// AnonMount is the public prefix for anonymous join: POST
// /api/v1/meet/anon/{code}/join-token with {"name": "...", "conn_id": "<random>"}.
const AnonMount = "/api/v1/meet/anon/"

// MatchAnon reports whether path is an anonymous join route and returns the code.
func MatchAnon(path string) (code string, ok bool) {
	if !strings.HasPrefix(path, AnonMount) {
		return "", false
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, AnonMount), "/"), "/")
	if len(parts) != 2 || parts[1] != "join-token" || !ValidCode(parts[0]) {
		return "", false
	}
	return parts[0], true
}

// ServeAnonymous lets a person without an account join a meeting through its
// link when the room's organization allows anonymous participants
// (meet.allow_anonymous). The visitor never joins directly: they always wait
// in the lobby until the host admits them, and they can never be host.
func (h *SFUHandler) ServeAnonymous(w http.ResponseWriter, r *http.Request) {
	code, ok := MatchAnon(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		jerr(w, http.StatusMethodNotAllowed, "method")
		return
	}
	if !h.cfg.Enabled() || h.repo == nil {
		jerr(w, http.StatusServiceUnavailable, "sfu_not_configured")
		return
	}
	if !h.anonLimit(r) {
		jerr(w, http.StatusTooManyRequests, "slow_down")
		return
	}
	var req joinReq
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)
	conn := PeerID("", req.ConnID)
	if len(conn) < 12 {
		jerr(w, http.StatusBadRequest, "conn_id_required")
		return
	}
	room, err := h.repo.GetByCodeAnyOrg(r.Context(), code)
	if err != nil {
		jerr(w, http.StatusNotFound, "room_not_found")
		return
	}
	if h.AnonAllowed == nil || !h.AnonAllowed(r.Context(), room.OrgID) {
		jerr(w, http.StatusForbidden, "anonymous_not_allowed")
		return
	}
	if room.Ended() {
		jerr(w, http.StatusGone, "meeting_ended")
		return
	}
	uid := "anon" + conn
	name := LKRoomName(room.OrgID, room.ID)
	h.mu.Lock()
	hostUser := h.state(name).hostID
	h.mu.Unlock()
	if hostUser == "" {
		hostUser = room.OwnerID
	}
	h.joinToken(w, r, name, room.ID, hostUser, uid, "Guest", false, true)
}

// anonLimit allows 60 anonymous join calls per minute per client address
// (the lobby poll is about one call per 3 seconds).
func (h *SFUHandler) anonLimit(r *http.Request) bool {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.IndexByte(ip, ','); i >= 0 {
		ip = ip[:i]
	}
	if ip = strings.TrimSpace(ip); ip == "" {
		ip = r.RemoteAddr
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.anonHits == nil {
		h.anonHits = map[string][]time.Time{}
	}
	cut := now.Add(-time.Minute)
	l := h.anonHits[ip][:0]
	for _, t := range h.anonHits[ip] {
		if t.After(cut) {
			l = append(l, t)
		}
	}
	if len(l) >= 60 {
		h.anonHits[ip] = l
		return false
	}
	h.anonHits[ip] = append(l, now)
	if len(h.anonHits) > 5000 {
		h.anonHits = map[string][]time.Time{ip: h.anonHits[ip]}
	}
	return true
}

// transcript handles /rooms/{id}/transcript:
//
//	POST   {"text": "..."}  any participant, only while the host has transcription on
//	GET    [?format=txt]    host only: the transcript as JSON or a text download
//	DELETE                  host only: erase the transcript
func (h *SFUHandler) transcript(w http.ResponseWriter, r *http.Request, name, orgID, roomID, uid, uname string, room Room, isHost bool) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		h.mu.Lock()
		on := h.state(name).transcribing
		banned := h.state(name).banned[uid]
		h.mu.Unlock()
		if !on || banned {
			jerr(w, http.StatusConflict, "transcription_off")
			return
		}
		var in struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil {
			jerr(w, http.StatusBadRequest, "bad_request")
			return
		}
		text, ok := CleanSegment(in.Text)
		if !ok {
			jerr(w, http.StatusBadRequest, "empty")
			return
		}
		stored, err := h.repo.AddSegment(ctx, orgID, roomID, uid, uname, text)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "store_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": stored})
	case http.MethodGet:
		if !isHost {
			jerr(w, http.StatusForbidden, "host_only")
			return
		}
		segs, err := h.repo.Segments(ctx, orgID, roomID)
		if err != nil {
			jerr(w, http.StatusInternalServerError, "read_failed")
			return
		}
		if r.URL.Query().Get("format") == "txt" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="meeting-transcript.txt"`)
			_, _ = w.Write([]byte(TranscriptText(room.Name, segs, nil)))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"segments": segs})
	case http.MethodDelete:
		if !isHost {
			jerr(w, http.StatusForbidden, "host_only")
			return
		}
		if err := h.repo.ClearSegments(ctx, orgID, roomID); err != nil {
			jerr(w, http.StatusInternalServerError, "clear_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		jerr(w, http.StatusMethodNotAllowed, "method")
	}
}
