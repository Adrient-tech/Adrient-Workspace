package podcasts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

// Effective is the voice setup an episode really uses after inheriting from its show.
type Effective struct {
	Provider     string  `json:"provider"` // provider id, or "browser"
	ProviderName string  `json:"provider_name"`
	Voice        string  `json:"voice"`
	VoiceMap     Voices  `json:"voice_map"`
	Speed        float64 `json:"speed"`
	Pitch        float64 `json:"pitch"`
	Language     string  `json:"language"`
	Mode         string  `json:"mode"` // "server" when a voice provider generates audio, else "browser"
}

// Override is what the episode itself sets (nil = inherit from the show).
type Override struct {
	Provider *string            `json:"provider"`
	Voice    *string            `json:"voice"`
	VoiceMap *map[string]string `json:"voice_map"`
	Speed    *float64           `json:"speed"`
	Pitch    *float64           `json:"pitch"`
}

// JobInfo describes the newest audio job of an episode.
type JobInfo struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Progress   int        `json:"progress"`
	Error      string     `json:"error,omitempty"`
	Chars      int        `json:"chars"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// EpisodeDetail is an episode with its text.
type EpisodeDetail struct {
	Episode
	BodyHTML      string         `json:"body_html"`
	Sentences     []tts.Sentence `json:"sentences"`
	Timings       []tts.Timing   `json:"timings"`
	AudioURL      string         `json:"audio_url,omitempty"`
	AudioMime     string         `json:"audio_mime,omitempty"`
	AudioStale    bool           `json:"audio_stale"`
	Effective     Effective      `json:"effective"`
	Override      Override       `json:"override"`
	Job           *JobInfo       `json:"job,omitempty"`
	CanEdit       bool           `json:"can_edit"`
	CommentsOn    bool           `json:"comments_on"`
	CanDownload   bool           `json:"can_download"`
	ShowDesc      string         `json:"show_description"`
	Audience      string         `json:"audience"`
	Chars         int            `json:"chars"`
	MaxChars      int            `json:"max_chars"`
	NextEpisodeID string         `json:"next_episode_id,omitempty"`
}

// plan is everything needed to narrate an episode.
type plan struct {
	org, showID, epID, author string
	title, bodyText           string
	sents                     []tts.Sentence
	eff                       Effective
	prov                      *ProviderRow
	hash                      string
	audioHash, audioStatus    string
	audioBlob, audioMime      string
	status                    string
	allowDownload             bool
	override                  Override
}

func (s *Stories) loadPlan(ctx context.Context, org, epID string) (*plan, error) {
	if !isUUID(epID) {
		return nil, ErrNotFound
	}
	var p plan
	var showProv, epProv *string
	var showVoice, epVoice string
	var showVM map[string]string
	var epVM map[string]string
	var showSpeed, showPitch float64
	var epSpeed, epPitch *float64
	var lang string
	err := s.Pool.QueryRow(ctx, `SELECT e.org_id::text, e.show_id::text, e.id::text, s.author_id::text, e.title, e.body_text, e.status,
		e.audio_hash, e.audio_status, e.audio_blob, e.audio_mime, s.allow_download, s.language,
		s.voice_provider::text, s.voice, s.voice_map, s.speed, s.pitch, e.voice_provider::text, e.voice, e.voice_map, e.speed, e.pitch
		FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id WHERE e.id=$1::uuid AND e.org_id=$2::uuid`, epID, org).
		Scan(&p.org, &p.showID, &p.epID, &p.author, &p.title, &p.bodyText, &p.status, &p.audioHash, &p.audioStatus, &p.audioBlob, &p.audioMime,
			&p.allowDownload, &lang, &showProv, &showVoice, &showVM, &showSpeed, &showPitch, &epProv, &epVoice, &epVM, &epSpeed, &epPitch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.override = Override{}
	if epProv != nil {
		x := providerName(epProv)
		p.override.Provider = &x
	}
	if epVoice != "" {
		p.override.Voice = &epVoice
	}
	if epVM != nil {
		p.override.VoiceMap = &epVM
	}
	p.override.Speed, p.override.Pitch = epSpeed, epPitch

	prov := providerName(showProv)
	if epProv != nil {
		prov = providerName(epProv)
	}
	voice := showVoice
	if epVoice != "" {
		voice = epVoice
	}
	vm := showVM
	if epVM != nil {
		vm = epVM
	}
	speed, pitch := showSpeed, showPitch
	if epSpeed != nil {
		speed = *epSpeed
	}
	if epPitch != nil {
		pitch = *epPitch
	}
	p.eff = Effective{Provider: "browser", Mode: "browser", Voice: voice, VoiceMap: Voices(vm), Speed: speed, Pitch: pitch, Language: lang}
	if p.eff.VoiceMap == nil {
		p.eff.VoiceMap = Voices{}
	}
	if prov != "browser" {
		var row ProviderRow
		var perr error
		if prov == "default" {
			row, perr = s.DefaultProvider(ctx, org)
		} else {
			row, perr = s.Provider(ctx, org, prov)
			if perr == nil && !row.Enabled {
				perr = ErrNotFound
			}
		}
		if perr == nil {
			p.prov = &row
			p.eff.Provider, p.eff.ProviderName, p.eff.Mode = row.ID, row.Name, "server"
			if p.eff.Voice == "" {
				p.eff.Voice = row.DefaultVoice
			}
		} else if !errors.Is(perr, ErrNotFound) {
			return nil, perr
		}
	}
	p.sents = tts.Prepare(p.bodyText)
	p.hash = p.computeHash()
	return &p, nil
}

func (p *plan) computeHash() string {
	h := sha256.New()
	speakers := map[string]string{}
	for _, s := range p.sents {
		if s.Speaker != "" {
			speakers[s.Speaker] = p.eff.VoiceMap[s.Speaker]
		}
	}
	keys := make([]string, 0, len(speakers))
	for k := range speakers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	prov, upd := "browser", int64(0)
	if p.prov != nil {
		prov, upd = p.prov.ID, p.prov.UpdatedAt.UnixMilli()
	}
	_ = json.NewEncoder(h).Encode(map[string]any{"v": 1, "s": p.sents, "p": prov, "u": upd, "voice": p.eff.Voice, "speed": p.eff.Speed,
		"pitch": p.eff.Pitch, "lang": p.eff.Language, "chars": keys, "map": speakers})
	return hex.EncodeToString(h.Sum(nil))
}

func (p *plan) chars() int {
	n := 0
	for _, s := range p.sents {
		n += len([]rune(s.Text))
	}
	return n
}

// EpisodeInput is what an author sets on an episode.
type EpisodeInput struct {
	Title    *string            `json:"title"`
	Summary  *string            `json:"summary"`
	BodyHTML *string            `json:"body_html"`
	Position *int               `json:"position"`
	Provider *string            `json:"provider"`
	Voice    *string            `json:"voice"`
	VoiceMap *map[string]string `json:"voice_map"`
	Speed    *float64           `json:"speed"`
	Pitch    *float64           `json:"pitch"`
	// Inherit lists override fields to clear ("provider", "voice", "voice_map", "speed", "pitch").
	Inherit []string `json:"inherit"`
}

func (s *Stories) checkEpisodeInput(in *EpisodeInput) error {
	if in.Title != nil {
		t := cleanText(*in.Title, 140)
		if t == "" {
			return invalid("an episode title is required")
		}
		in.Title = &t
	}
	if in.Summary != nil {
		x := cleanText(*in.Summary, 2000)
		in.Summary = &x
	}
	if in.BodyHTML != nil {
		if len(*in.BodyHTML) > 1_500_000 {
			return invalid("the story text is too long")
		}
		x := SanitizeHTML(*in.BodyHTML)
		in.BodyHTML = &x
	}
	if in.Speed != nil && (*in.Speed < 0.5 || *in.Speed > 2) {
		return invalid("speed must be between 0.5 and 2")
	}
	if in.Pitch != nil && (*in.Pitch < 0.5 || *in.Pitch > 1.5) {
		return invalid("pitch must be between 0.5 and 1.5")
	}
	if in.Voice != nil && *in.Voice != "" && !tts.ValidVoice(*in.Voice) {
		return invalid("voice id is not valid")
	}
	if in.VoiceMap != nil {
		if len(*in.VoiceMap) > 12 {
			return invalid("at most 12 character voices")
		}
		for k, v := range *in.VoiceMap {
			if len(k) > 40 || (v != "" && !tts.ValidVoice(v)) {
				return invalid("character voice is not valid")
			}
		}
	}
	return nil
}

// CreateEpisode adds an episode (a draft) at the end of a show.
func (s *Stories) CreateEpisode(ctx context.Context, v Viewer, showID string, in EpisodeInput) (EpisodeDetail, error) {
	if err := s.authorOnly(ctx, v, showID); err != nil {
		return EpisodeDetail{}, err
	}
	if in.Title == nil {
		t := "Untitled episode"
		in.Title = &t
	}
	if err := s.checkEpisodeInput(&in); err != nil {
		return EpisodeDetail{}, err
	}
	html := val(in.BodyHTML, "")
	text := tts.HTMLToText(html)
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.story_episodes (org_id, show_id, position, title, summary, body_html, body_text, est_ms)
		VALUES ($1::uuid,$2::uuid,(SELECT COALESCE(max(position),0)+1 FROM adrient.story_episodes WHERE show_id=$2::uuid),$3,$4,$5,$6,$7) RETURNING id::text`,
		v.OrgID, showID, *in.Title, val(in.Summary, ""), html, text, tts.EstimateMs(tts.Prepare(text), 1)).Scan(&id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET updated_at=now() WHERE id=$1::uuid`, showID); err != nil {
		return EpisodeDetail{}, err
	}
	return s.GetEpisode(ctx, v, id)
}

// UpdateEpisode changes the fields that are set. A published episode whose text
// or voice changed gets new audio (when a server voice is in use).
func (s *Stories) UpdateEpisode(ctx context.Context, v Viewer, id string, in EpisodeInput) (EpisodeDetail, error) {
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	if p.author != v.UserID {
		return EpisodeDetail{}, forbidden("only the author can change this story")
	}
	if err := s.checkEpisodeInput(&in); err != nil {
		return EpisodeDetail{}, err
	}
	sets := []string{"updated_at=now()"}
	args := []any{id}
	set := func(col string, a any) {
		args = append(args, a)
		sets = append(sets, col+"=$"+itoa(len(args)))
	}
	if in.Title != nil {
		set("title", *in.Title)
	}
	if in.Summary != nil {
		set("summary", *in.Summary)
	}
	if in.Position != nil && *in.Position > 0 {
		set("position", *in.Position)
	}
	speedForEst := p.eff.Speed
	if in.BodyHTML != nil {
		text := tts.HTMLToText(*in.BodyHTML)
		set("body_html", *in.BodyHTML)
		set("body_text", text)
		set("est_ms", tts.EstimateMs(tts.Prepare(text), speedForEst))
	}
	if in.Provider != nil {
		a, err := providerArg(*in.Provider)
		if err != nil {
			return EpisodeDetail{}, err
		}
		set("voice_provider", a)
	}
	if in.Voice != nil {
		set("voice", *in.Voice)
	}
	if in.VoiceMap != nil {
		set("voice_map", *in.VoiceMap)
	}
	if in.Speed != nil {
		set("speed", *in.Speed)
	}
	if in.Pitch != nil {
		set("pitch", *in.Pitch)
	}
	for _, f := range in.Inherit {
		switch f {
		case "provider":
			sets = append(sets, "voice_provider=NULL")
		case "voice":
			sets = append(sets, "voice=''")
		case "voice_map":
			sets = append(sets, "voice_map=NULL")
		case "speed":
			sets = append(sets, "speed=NULL")
		case "pitch":
			sets = append(sets, "pitch=NULL")
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET `+strings.Join(sets, ",")+` WHERE id=$1::uuid`, args...); err != nil {
		return EpisodeDetail{}, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET updated_at=now() WHERE id=$1::uuid`, p.showID)
	if p.status == "published" {
		s.autoAudio(ctx, v, id)
	}
	return s.GetEpisode(ctx, v, id)
}

// autoAudio queues new audio for a published episode when it is missing or out of date.
// It is best effort: limits and missing voices are not errors here.
func (s *Stories) autoAudio(ctx context.Context, v Viewer, id string) {
	if !s.GetSettings(ctx, v.OrgID).AutoGenerate {
		return
	}
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil || p.prov == nil || len(p.sents) == 0 {
		return
	}
	if p.audioStatus == "ready" && p.audioHash == p.hash {
		return
	}
	if _, err := s.RequestAudio(ctx, v, id, false); err != nil && !errors.Is(err, ErrLimit) && !errors.Is(err, ErrInvalid) {
		s.log("auto audio", err)
	}
}

// DeleteEpisode removes an episode and its audio.
func (s *Stories) DeleteEpisode(ctx context.Context, v Viewer, id string) error {
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil {
		return err
	}
	if p.author != v.UserID {
		return forbidden("only the author can delete this story")
	}
	_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.story_library WHERE kind='episode' AND target_id=$1::uuid`, id)
	if _, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_episodes WHERE id=$1::uuid AND org_id=$2::uuid`, id, v.OrgID); err != nil {
		return err
	}
	if p.audioBlob != "" {
		s.dropBlobs(ctx, v.OrgID, []string{p.audioBlob})
	}
	return nil
}

// ListEpisodes returns the episodes of a show (all of them for its author, published ones for others).
func (s *Stories) ListEpisodes(ctx context.Context, v Viewer, showID string) ([]Episode, error) {
	if !isUUID(showID) {
		return nil, ErrNotFound
	}
	if _, err := s.GetShow(ctx, v, showID); err != nil {
		return nil, err
	}
	return s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE e.show_id=$5::uuid AND `+visible+` AND (e.status='published' OR s.author_id=$2::uuid)
		ORDER BY e.position, e.created_at`, v.args(showID)...)
}

// GetEpisode returns an episode with its text, timings and voice setup.
func (s *Stories) GetEpisode(ctx context.Context, v Viewer, id string) (EpisodeDetail, error) {
	if !isUUID(id) {
		return EpisodeDetail{}, ErrNotFound
	}
	list, err := s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE e.id=$5::uuid AND `+visible+` AND (e.status='published' OR s.author_id=$2::uuid)`, v.args(id)...)
	if err != nil {
		return EpisodeDetail{}, err
	}
	if len(list) == 0 {
		return EpisodeDetail{}, ErrNotFound
	}
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	d := EpisodeDetail{Episode: list[0], Sentences: p.sents, Effective: p.eff, Override: p.override, CanEdit: p.author == v.UserID, MaxChars: s.GetSettings(ctx, v.OrgID).MaxChars}
	if d.Sentences == nil {
		d.Sentences = []tts.Sentence{}
	}
	d.Chars = p.chars()
	var timings []byte
	var showDesc string
	var commentsOn bool
	if err := s.Pool.QueryRow(ctx, `SELECT e.body_html, e.audio_timings, e.audio_mime, sh.description, sh.comments_on, sh.audience
		FROM adrient.story_episodes e JOIN adrient.story_shows sh ON sh.id=e.show_id WHERE e.id=$1::uuid`, id).
		Scan(&d.BodyHTML, &timings, &d.AudioMime, &showDesc, &commentsOn, &d.Audience); err != nil {
		return d, err
	}
	d.ShowDesc, d.CommentsOn = showDesc, commentsOn
	_ = json.Unmarshal(timings, &d.Timings)
	if d.Timings == nil {
		d.Timings = []tts.Timing{}
	}
	if d.HasAudio {
		d.AudioURL = "/api/v1/stories/episodes/" + id + "/audio?v=" + p.audioHash[:min(8, len(p.audioHash))]
		d.AudioStale = p.audioHash != p.hash
	}
	d.CanDownload = p.allowDownload && d.HasAudio && authzDownload(ctx)
	if d.CanEdit {
		d.CanDownload = d.HasAudio
		d.Job = s.latestJob(ctx, id)
	}
	_ = s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.story_episodes WHERE show_id=$1::uuid AND status='published' AND (position,created_at) > (SELECT position,created_at FROM adrient.story_episodes WHERE id=$2::uuid)
		ORDER BY position, created_at LIMIT 1`, p.showID, id).Scan(&d.NextEpisodeID)
	return d, nil
}

// authzDownload is set by the HTTP layer through the request context (control stories.download).
func authzDownload(ctx context.Context) bool { v, _ := ctx.Value(downloadKey{}).(bool); return v }

type downloadKey struct{}

// WithDownload marks whether the caller may download audio (control result).
func WithDownload(ctx context.Context, ok bool) context.Context {
	return context.WithValue(ctx, downloadKey{}, ok)
}

func (s *Stories) latestJob(ctx context.Context, epID string) *JobInfo {
	var j JobInfo
	err := s.Pool.QueryRow(ctx, `SELECT id::text, status, progress, error, chars, created_at, finished_at FROM adrient.story_audio_jobs WHERE episode_id=$1::uuid ORDER BY created_at DESC LIMIT 1`, epID).
		Scan(&j.ID, &j.Status, &j.Progress, &j.Error, &j.Chars, &j.CreatedAt, &j.FinishedAt)
	if err != nil {
		return nil
	}
	return &j
}

// ---- publishing ---------------------------------------------------------------

// Publish makes an episode visible now, or at a later time when at is in the future.
func (s *Stories) Publish(ctx context.Context, v Viewer, id string, at *time.Time) (EpisodeDetail, error) {
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	if p.author != v.UserID {
		return EpisodeDetail{}, forbidden("only the author can publish this story")
	}
	if strings.TrimSpace(p.title) == "" || len(p.sents) == 0 {
		return EpisodeDetail{}, invalid("write some text before publishing")
	}
	if at != nil && at.After(s.now().Add(time.Minute)) {
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET status='scheduled', publish_at=$2, updated_at=now() WHERE id=$1::uuid`, id, *at); err != nil {
			return EpisodeDetail{}, err
		}
		return s.GetEpisode(ctx, v, id)
	}
	if err := s.publishNow(ctx, p); err != nil {
		return EpisodeDetail{}, err
	}
	s.autoAudio(ctx, v, id)
	return s.GetEpisode(ctx, v, id)
}

func (s *Stories) publishNow(ctx context.Context, p *plan) error {
	var first bool
	err := s.Pool.QueryRow(ctx, `UPDATE adrient.story_episodes SET status='published', publish_at=NULL, published_at=COALESCE(published_at, $2), updated_at=now()
		WHERE id=$1::uuid RETURNING published_at = $2`, p.epID, s.now()).Scan(&first)
	if err != nil {
		return err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_shows SET updated_at=now() WHERE id=$1::uuid`, p.showID)
	if first {
		go s.notifyPublished(context.WithoutCancel(ctx), p)
	}
	return nil
}

// Unpublish hides an episode again (and cancels a schedule).
func (s *Stories) Unpublish(ctx context.Context, v Viewer, id string) (EpisodeDetail, error) {
	p, err := s.loadPlan(ctx, v.OrgID, id)
	if err != nil {
		return EpisodeDetail{}, err
	}
	if p.author != v.UserID {
		return EpisodeDetail{}, forbidden("only the author can unpublish this story")
	}
	status := "unpublished"
	if p.status == "scheduled" || p.status == "draft" {
		status = "draft"
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET status=$2, publish_at=NULL, updated_at=now() WHERE id=$1::uuid`, id, status); err != nil {
		return EpisodeDetail{}, err
	}
	return s.GetEpisode(ctx, v, id)
}

// publishDue publishes scheduled episodes whose time has come.
func (s *Stories) publishDue(ctx context.Context) {
	rows, err := s.Pool.Query(ctx, `SELECT org_id::text, id::text, (SELECT author_id::text FROM adrient.story_shows WHERE id=show_id) FROM adrient.story_episodes
		WHERE status='scheduled' AND publish_at <= $1 LIMIT 50`, s.now())
	if err != nil {
		s.log("scheduler", err)
		return
	}
	type due struct{ org, id, author string }
	var list []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.org, &d.id, &d.author) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	for _, d := range list {
		p, err := s.loadPlan(ctx, d.org, d.id)
		if err != nil {
			s.log("scheduler plan", err)
			continue
		}
		if err := s.publishNow(ctx, p); err != nil {
			s.log("scheduler publish", err)
			continue
		}
		if s.Audit != nil {
			s.Audit(ctx, d.org, d.author, "story.published_scheduled", d.id, map[string]any{"title": p.title})
		}
		s.autoAudio(ctx, Viewer{OrgID: d.org, UserID: d.author}, d.id)
	}
}

// notifyPublished tells the followers of the show and of its author.
func (s *Stories) notifyPublished(ctx context.Context, p *plan) {
	if s.Notify == nil {
		return
	}
	var showTitle, authorName, audience string
	var audID *string
	if err := s.Pool.QueryRow(ctx, `SELECT s.title, u.display_name, s.audience, s.audience_id::text FROM adrient.story_shows s JOIN adrient.users u ON u.id=s.author_id WHERE s.id=$1::uuid`, p.showID).
		Scan(&showTitle, &authorName, &audience, &audID); err != nil {
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT user_id::text FROM adrient.story_follows WHERE org_id=$1::uuid AND user_id<>$4::uuid
		AND ((kind='show' AND target_id=$2::uuid) OR (kind='author' AND target_id=$3::uuid))`, p.org, p.showID, p.author, p.author)
	if err != nil {
		return
	}
	var users []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			users = append(users, u)
		}
	}
	rows.Close()
	for _, u := range users {
		if audience != "everyone" {
			vw, err := s.LoadViewer(ctx, p.org, u, false)
			if err != nil {
				continue
			}
			var ok bool
			_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.story_shows s WHERE s.id=$5::uuid AND `+visible+`)`, vw.args(p.showID)...).Scan(&ok)
			if !ok {
				continue
			}
		}
		s.Notify(ctx, p.org, u, p.author, "story_episode", "New episode: "+p.title, fmt.Sprintf("%s · %s", showTitle, authorName), "/podcasts/episode/"+p.epID)
	}
}
