package podcasts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

// StoriesMount is where the Stories API lives.
const StoriesMount = "/api/v1/stories"

// StoriesHTTP serves /api/v1/stories/*.
type StoriesHTTP struct {
	S *Stories
	// IsAdmin tells whether the caller is an organisation administrator.
	IsAdmin func(r *http.Request) bool
}

// Match reports whether the path belongs to the Stories API.
func (h *StoriesHTTP) Match(path string) bool {
	return path == StoriesMount || strings.HasPrefix(path, StoriesMount+"/")
}

func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrForbidden):
		http.Error(w, strings.TrimPrefix(err.Error(), "forbidden: "), http.StatusForbidden)
	case errors.Is(err, ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.Is(err, ErrConflict):
		http.Error(w, strings.TrimPrefix(err.Error(), "conflict: "), http.StatusConflict)
	case errors.Is(err, ErrLimit):
		http.Error(w, strings.TrimPrefix(err.Error(), "limit: "), http.StatusTooManyRequests)
	default:
		slog.Error("stories: request failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true // empty body: all fields optional
		}
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (h *StoriesHTTP) viewer(r *http.Request) (Viewer, error) {
	o, u, ok := authCtx(r)
	if !ok {
		return Viewer{}, errors.New("unauthorized")
	}
	admin := h.IsAdmin != nil && h.IsAdmin(r)
	return h.S.LoadViewer(r.Context(), o, u, admin)
}

// ServeHTTP routes the request.
func (h *StoriesHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := authCtx(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	v, err := h.viewer(r)
	if err != nil {
		fail(w, err)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, StoriesMount), "/")
	seg := strings.Split(rest, "/")
	for len(seg) < 4 {
		seg = append(seg, "")
	}
	m := r.Method
	ctx := r.Context()
	switch {
	case seg[0] == "home" && m == http.MethodGet:
		x, err := h.S.Home(ctx, v)
		reply(w, x, err)
	case seg[0] == "search" && m == http.MethodGet:
		x, err := h.S.Search(ctx, v, r.URL.Query().Get("q"))
		reply(w, x, err)
	case seg[0] == "genres" && m == http.MethodGet:
		jsonOut(w, 200, map[string]any{"genres": Genres})
	case seg[0] == "audiences" && m == http.MethodGet:
		x, err := h.S.Audiences(ctx, v.OrgID)
		reply(w, map[string]any{"audiences": x}, err)
	case seg[0] == "voices" && m == http.MethodGet:
		x, err := h.S.Voices(ctx, v.OrgID)
		reply(w, map[string]any{"providers": x, "browser_available": true}, err)
	case seg[0] == "quota" && m == http.MethodGet:
		q := h.S.QuotaFor(ctx, v.OrgID, v.UserID)
		jsonOut(w, 200, map[string]any{"quota": q, "can_publish": authz.ControlBool(ctx, "stories.publish"), "can_generate": authz.ControlBool(ctx, "stories.generate_audio")})
	case seg[0] == "library" && seg[1] == "" && m == http.MethodGet:
		x, err := h.S.MyLibrary(ctx, v)
		reply(w, x, err)
	case seg[0] == "library" && seg[2] != "" && (m == http.MethodPut || m == http.MethodDelete):
		reply(w, map[string]bool{"ok": true}, h.S.SetLibrary(ctx, v, seg[1], seg[2], m == http.MethodPut))
	case seg[0] == "follows" && seg[1] == "" && m == http.MethodGet:
		x, err := h.S.FollowedAuthors(ctx, v)
		reply(w, map[string]any{"authors": x}, err)
	case seg[0] == "follows" && seg[2] != "" && (m == http.MethodPut || m == http.MethodDelete):
		reply(w, map[string]bool{"ok": true}, h.S.SetFollow(ctx, v, seg[1], seg[2], m == http.MethodPut))
	case seg[0] == "moderation" && m == http.MethodGet:
		x, err := h.S.ReportedComments(ctx, v)
		reply(w, map[string]any{"comments": x}, err)
	case seg[0] == "comments" && seg[1] != "" && seg[2] != "" && m == http.MethodPost:
		h.comment(w, r, v, seg[1], seg[2])
	case seg[0] == "shows":
		h.shows(w, r, v, seg)
	case seg[0] == "episodes" && seg[1] != "":
		h.episodes(w, r, v, seg)
	case seg[0] == "admin":
		h.admin(w, r, v, seg)
	default:
		http.NotFound(w, r)
	}
}

func reply(w http.ResponseWriter, v any, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, v)
}

func (h *StoriesHTTP) comment(w http.ResponseWriter, r *http.Request, v Viewer, id, action string) {
	if action == "report" {
		var b struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &b) {
			return
		}
		reply(w, map[string]bool{"ok": true}, h.S.ReportComment(r.Context(), v, id, b.Reason))
		return
	}
	reply(w, map[string]bool{"ok": true}, h.S.ModerateComment(r.Context(), v, id, action))
}

func (h *StoriesHTTP) shows(w http.ResponseWriter, r *http.Request, v Viewer, seg []string) {
	ctx, m := r.Context(), r.Method
	id, sub := seg[1], seg[2]
	switch {
	case id == "" && m == http.MethodGet:
		q := r.URL.Query()
		x, err := h.S.ListShows(ctx, v, ShowFilter{Genre: q.Get("genre"), Q: q.Get("q"), Sort: q.Get("sort"), Mine: q.Get("mine") == "1",
			Following: q.Get("following") == "1", Library: q.Get("library") == "1"})
		reply(w, map[string]any{"shows": x}, err)
	case id == "" && m == http.MethodPost:
		if !authz.ControlBool(ctx, "stories.publish") {
			http.Error(w, "your organisation does not allow you to write stories", http.StatusForbidden)
			return
		}
		var in ShowInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.CreateShow(ctx, v, in)
		reply(w, x, err)
	case sub == "" && m == http.MethodGet:
		x, err := h.S.GetShow(ctx, v, id)
		if err != nil {
			fail(w, err)
			return
		}
		eps, err := h.S.ListEpisodes(ctx, v, id)
		reply(w, map[string]any{"show": x, "episodes": eps}, err)
	case sub == "" && m == http.MethodPatch:
		var in ShowInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.UpdateShow(ctx, v, id, in)
		reply(w, x, err)
	case sub == "" && m == http.MethodDelete:
		reply(w, map[string]bool{"ok": true}, h.S.DeleteShow(ctx, v, id))
	case sub == "episodes" && m == http.MethodGet:
		x, err := h.S.ListEpisodes(ctx, v, id)
		reply(w, map[string]any{"episodes": x}, err)
	case sub == "episodes" && m == http.MethodPost:
		if !authz.ControlBool(ctx, "stories.publish") {
			http.Error(w, "your organisation does not allow you to write stories", http.StatusForbidden)
			return
		}
		var in EpisodeInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.CreateEpisode(ctx, v, id, in)
		reply(w, x, err)
	case sub == "cover" && m == http.MethodGet:
		rc, mime, err := h.S.Cover(ctx, v, id)
		if err != nil {
			fail(w, err)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, rc)
	case sub == "cover" && m == http.MethodPut:
		x, err := h.S.SetCover(ctx, v, id, http.MaxBytesReader(w, r.Body, 4<<20))
		reply(w, x, err)
	case sub == "cover" && m == http.MethodDelete:
		x, err := h.S.ClearCover(ctx, v, id)
		reply(w, x, err)
	case sub == "stats" && m == http.MethodGet:
		x, err := h.S.Stats(ctx, v, id)
		reply(w, x, err)
	default:
		http.NotFound(w, r)
	}
}

func (h *StoriesHTTP) episodes(w http.ResponseWriter, r *http.Request, v Viewer, seg []string) {
	ctx, m := r.Context(), r.Method
	id, sub := seg[1], seg[2]
	switch {
	case sub == "" && m == http.MethodGet:
		ctx = WithDownload(ctx, authz.ControlBool(ctx, "stories.download"))
		x, err := h.S.GetEpisode(ctx, v, id)
		reply(w, x, err)
	case sub == "" && m == http.MethodPatch:
		var in EpisodeInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.UpdateEpisode(ctx, v, id, in)
		reply(w, x, err)
	case sub == "" && m == http.MethodDelete:
		reply(w, map[string]bool{"ok": true}, h.S.DeleteEpisode(ctx, v, id))
	case sub == "publish" && m == http.MethodPost:
		if !authz.ControlBool(ctx, "stories.publish") {
			http.Error(w, "your organisation does not allow you to publish stories", http.StatusForbidden)
			return
		}
		var b struct {
			At *time.Time `json:"at"`
		}
		if !decode(w, r, &b) {
			return
		}
		x, err := h.S.Publish(ctx, v, id, b.At)
		reply(w, x, err)
	case sub == "unpublish" && m == http.MethodPost:
		x, err := h.S.Unpublish(ctx, v, id)
		reply(w, x, err)
	case sub == "audio" && m == http.MethodPost:
		if !authz.ControlBool(ctx, "stories.generate_audio") {
			http.Error(w, "your organisation does not allow generating audio", http.StatusForbidden)
			return
		}
		var b struct {
			Force bool `json:"force"`
		}
		if !decode(w, r, &b) {
			return
		}
		job, err := h.S.RequestAudio(ctx, v, id, b.Force)
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusAccepted, map[string]any{"job": job})
	case sub == "audio" && (m == http.MethodGet || m == http.MethodHead):
		download := r.URL.Query().Get("download") == "1"
		af, err := h.S.OpenAudio(ctx, v, id, download, authz.ControlBool(ctx, "stories.download"))
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", af.Mime)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Header().Set("ETag", `"`+af.Hash+`"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if download {
			w.Header().Set("Content-Disposition", `attachment; filename="`+af.Name+`"`)
		}
		http.ServeContent(w, r, af.Name, af.Modified, bytes.NewReader(af.Data))
	case sub == "progress" && m == http.MethodPut:
		var p Progress
		if !decode(w, r, &p) {
			return
		}
		reply(w, map[string]bool{"ok": true}, h.S.SaveProgress(ctx, v, id, p))
	case sub == "play" && m == http.MethodPost:
		reply(w, map[string]bool{"ok": true}, h.S.RecordPlay(ctx, v, id))
	case sub == "played" && (m == http.MethodPut || m == http.MethodDelete):
		reply(w, map[string]bool{"ok": true}, h.S.MarkPlayed(ctx, v, id, m == http.MethodPut))
	case sub == "like" && (m == http.MethodPut || m == http.MethodDelete):
		reply(w, map[string]bool{"ok": true}, h.S.SetLike(ctx, v, id, m == http.MethodPut))
	case sub == "comments" && m == http.MethodGet:
		x, err := h.S.ListComments(ctx, v, id)
		reply(w, map[string]any{"comments": x}, err)
	case sub == "comments" && m == http.MethodPost:
		var b struct {
			Body string `json:"body"`
		}
		if !decode(w, r, &b) {
			return
		}
		x, err := h.S.AddComment(ctx, v, id, b.Body)
		reply(w, x, err)
	default:
		http.NotFound(w, r)
	}
}

func (h *StoriesHTTP) admin(w http.ResponseWriter, r *http.Request, v Viewer, seg []string) {
	if !v.Admin {
		http.Error(w, "administrators only", http.StatusForbidden)
		return
	}
	ctx, m := r.Context(), r.Method
	switch {
	case seg[1] == "providers" && seg[2] == "" && m == http.MethodGet:
		x, err := h.S.ListProviders(ctx, v.OrgID)
		reply(w, map[string]any{"providers": x}, err)
	case seg[1] == "providers" && seg[2] == "" && m == http.MethodPost:
		var in ProviderInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.SaveProvider(ctx, v.OrgID, "", in)
		reply(w, x, err)
	case seg[1] == "providers" && seg[2] != "" && seg[3] == "" && m == http.MethodPut:
		var in ProviderInput
		if !decode(w, r, &in) {
			return
		}
		x, err := h.S.SaveProvider(ctx, v.OrgID, seg[2], in)
		reply(w, x, err)
	case seg[1] == "providers" && seg[2] != "" && seg[3] == "" && m == http.MethodDelete:
		reply(w, map[string]bool{"ok": true}, h.S.DeleteProvider(ctx, v.OrgID, seg[2]))
	case seg[1] == "providers" && seg[2] != "" && seg[3] == "test" && m == http.MethodPost:
		var b struct {
			Voice string `json:"voice"`
		}
		if !decode(w, r, &b) {
			return
		}
		jsonOut(w, 200, h.S.TestProvider(ctx, v.OrgID, seg[2], b.Voice))
	case seg[1] == "detect" && m == http.MethodGet:
		jsonOut(w, 200, map[string]any{"presets": tts.Detect()})
	case seg[1] == "settings" && m == http.MethodGet:
		jsonOut(w, 200, h.S.GetSettings(ctx, v.OrgID))
	case seg[1] == "settings" && m == http.MethodPut:
		var st Settings
		if !decode(w, r, &st) {
			return
		}
		x, err := h.S.SaveSettings(ctx, v.OrgID, st)
		reply(w, x, err)
	case seg[1] == "jobs" && m == http.MethodGet:
		x, err := h.S.ListJobs(ctx, v.OrgID)
		reply(w, map[string]any{"jobs": x}, err)
	default:
		http.NotFound(w, r)
	}
}

// compile-time check that the auth helpers used above exist
var _ = auth.OrgFromContext
