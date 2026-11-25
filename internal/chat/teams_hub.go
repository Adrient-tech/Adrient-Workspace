package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/coder/websocket"
)

// userPeer is one browser tab's live-feed connection.
type userPeer struct {
	userID string
	orgID  string
	away   bool
	out    chan []byte
}

// feedEnvelope is the JSON sent over the per-user feed.
//
//	{"type":"message","channel_id","message","notify":[ids]}
//	{"type":"edited"|"deleted"|"reaction"|"pins"|"channel"|"sections"|"reminder"|"read"}
//	{"type":"presence","online":[ids],"away":[ids]}
//
// Client -> server: {"type":"away","away":true|false}.
type feedEnvelope map[string]any

func (h *Hub) usersLocked() map[*userPeer]struct{} {
	if h.users == nil {
		h.users = map[*userPeer]struct{}{}
	}
	return h.users
}

// NotifyUsers sends payload to every live tab of the given users.
func (h *Hub) NotifyUsers(userIDs []string, env feedEnvelope) {
	if h == nil {
		return
	}
	want := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		want[id] = true
	}
	b, _ := json.Marshal(env)
	h.umu.Lock()
	targets := make([]*userPeer, 0, 8)
	for p := range h.usersLocked() {
		if want[p.userID] {
			targets = append(targets, p)
		}
	}
	h.umu.Unlock()
	for _, p := range targets {
		select {
		case p.out <- b:
		default:
		}
	}
}

// Publish sends an event to every live tab of the given users. It is the
// workspace-wide push channel (notifications, call rings) used by other apps.
func (h *Hub) Publish(userIDs []string, event map[string]any) {
	h.NotifyUsers(userIDs, feedEnvelope(event))
}

// PublishOrg sends an event to every live tab of every user in the org.
func (h *Hub) PublishOrg(orgID string, event map[string]any) {
	if h == nil {
		return
	}
	b, _ := json.Marshal(event)
	h.umu.Lock()
	targets := make([]*userPeer, 0, 16)
	for p := range h.usersLocked() {
		if p.orgID == orgID {
			targets = append(targets, p)
		}
	}
	h.umu.Unlock()
	for _, p := range targets {
		select {
		case p.out <- b:
		default:
		}
	}
}

// PresenceOf returns the online and away user ids of an org.
func (h *Hub) PresenceOf(orgID string) (online, away []string) {
	h.umu.Lock()
	defer h.umu.Unlock()
	active := map[string]bool{}
	seen := map[string]bool{}
	for p := range h.usersLocked() {
		if p.orgID != orgID {
			continue
		}
		seen[p.userID] = true
		if !p.away {
			active[p.userID] = true
		}
	}
	online, away = []string{}, []string{}
	for id := range seen {
		if active[id] {
			online = append(online, id)
		} else {
			away = append(away, id)
		}
	}
	sort.Strings(online)
	sort.Strings(away)
	return
}

func (h *Hub) broadcastPresenceOrg(orgID string) {
	online, away := h.PresenceOf(orgID)
	b, _ := json.Marshal(feedEnvelope{"type": "presence", "online": online, "away": away})
	h.umu.Lock()
	targets := make([]*userPeer, 0, 8)
	for p := range h.usersLocked() {
		if p.orgID == orgID {
			targets = append(targets, p)
		}
	}
	h.umu.Unlock()
	for _, p := range targets {
		select {
		case p.out <- b:
		default:
		}
	}
}

// ServeUser handles one per-user feed WebSocket. The caller has authenticated
// the request; every event is addressed by user id so no per-channel check is
// needed on the socket itself.
func (h *Hub) ServeUser(w http.ResponseWriter, r *http.Request, userID, orgID string) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(wsReadLimit)
	ctx := r.Context()
	self := &userPeer{userID: userID, orgID: orgID, out: make(chan []byte, 256)}
	h.umu.Lock()
	h.usersLocked()[self] = struct{}{}
	h.umu.Unlock()
	h.broadcastPresenceOrg(orgID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-self.out:
				wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.Write(wctx, websocket.MessageText, msg)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		var in struct {
			Type string `json:"type"`
			Away bool   `json:"away"`
		}
		if json.Unmarshal(data, &in) == nil && in.Type == "away" {
			h.umu.Lock()
			changed := self.away != in.Away
			self.away = in.Away
			h.umu.Unlock()
			if changed {
				h.broadcastPresenceOrg(orgID)
			}
		}
	}
	h.umu.Lock()
	delete(h.usersLocked(), self)
	h.umu.Unlock()
	h.broadcastPresenceOrg(orgID)
	<-done
}
