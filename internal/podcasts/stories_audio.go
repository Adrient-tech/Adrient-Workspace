package podcasts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

// Quota is what a user may still generate today.
type Quota struct {
	UsedChars     int `json:"used_chars"`
	DailyChars    int `json:"daily_chars"`
	PerMinuteJobs int `json:"per_minute_jobs"`
	MaxChars      int `json:"max_chars"`
}

// QuotaFor reports the user's generation use in the last 24 hours.
func (s *Stories) QuotaFor(ctx context.Context, org, user string) Quota {
	st := s.GetSettings(ctx, org)
	q := Quota{DailyChars: st.DailyChars, PerMinuteJobs: st.PerMinuteJobs, MaxChars: st.MaxChars}
	_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(sum(chars),0) FROM adrient.story_audio_jobs WHERE org_id=$1::uuid AND user_id=$2::uuid
		AND created_at > now() - interval '24 hours' AND status IN ('queued','running','done')`, org, user).Scan(&q.UsedChars)
	return q
}

// RequestAudio queues generation of an episode's audio and returns at once; a
// worker does the work. Identical text with the same voice settings reuses the
// audio already stored for it (no job, no quota). force regenerates anyway.
func (s *Stories) RequestAudio(ctx context.Context, v Viewer, epID string, force bool) (*JobInfo, error) {
	p, err := s.loadPlan(ctx, v.OrgID, epID)
	if err != nil {
		return nil, err
	}
	if p.author != v.UserID {
		return nil, forbidden("only the author can generate audio for this story")
	}
	if p.prov == nil {
		return nil, invalid("no server voice is set up, so listeners hear this story read aloud by their own browser. An administrator can add voices in Admin > Voices")
	}
	if len(p.sents) == 0 {
		return nil, invalid("write some text first")
	}
	st := s.GetSettings(ctx, v.OrgID)
	chars := p.chars()
	if chars > st.MaxChars {
		return nil, invalid("this episode has %d characters; the limit for audio is %d. Split it into two episodes", chars, st.MaxChars)
	}
	if !force {
		if p.audioStatus == "ready" && p.audioHash == p.hash {
			return s.latestJob(ctx, epID), nil
		}
		if ok, err := s.reuseCached(ctx, p); err != nil {
			return nil, err
		} else if ok {
			return s.latestJob(ctx, epID), nil
		}
	}
	// one job at a time per episode
	var existing string
	if err := s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.story_audio_jobs WHERE episode_id=$1::uuid AND status IN ('queued','running') LIMIT 1`, epID).Scan(&existing); err == nil {
		return s.latestJob(ctx, epID), nil
	}
	q := s.QuotaFor(ctx, v.OrgID, v.UserID)
	if q.DailyChars > 0 && q.UsedChars+chars > q.DailyChars {
		return nil, limit("daily audio limit reached (%d of %d characters used in the last 24 hours)", q.UsedChars, q.DailyChars)
	}
	var recent int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.story_audio_jobs WHERE org_id=$1::uuid AND user_id=$2::uuid AND created_at > now() - interval '1 minute'`, v.OrgID, v.UserID).Scan(&recent)
	if recent >= st.PerMinuteJobs {
		return nil, limit("too many audio requests, try again in a minute")
	}
	var jobID string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.story_audio_jobs (org_id, episode_id, user_id, chars) VALUES ($1::uuid,$2::uuid,$3::uuid,$4) RETURNING id::text`,
		v.OrgID, epID, v.UserID, chars).Scan(&jobID); err != nil {
		return nil, err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET audio_status=CASE WHEN audio_blob<>'' THEN audio_status ELSE 'queued' END, audio_error='' WHERE id=$1::uuid`, epID); err != nil {
		return nil, err
	}
	if s.Audit != nil {
		s.Audit(ctx, v.OrgID, v.UserID, "story.audio_requested", epID, map[string]any{"chars": chars, "provider": p.prov.Name, "job": jobID})
	}
	select {
	case s.kick <- struct{}{}:
	default:
	}
	return s.latestJob(ctx, epID), nil
}

// reuseCached points the episode at audio already generated for the same hash.
func (s *Stories) reuseCached(ctx context.Context, p *plan) (bool, error) {
	var blob, mime string
	var bytesN int64
	var ms int
	var timings []byte
	err := s.Pool.QueryRow(ctx, `SELECT audio_blob, audio_mime, audio_bytes, audio_ms, audio_timings FROM adrient.story_episodes
		WHERE org_id=$1::uuid AND audio_hash=$2 AND audio_status='ready' AND audio_blob<>'' LIMIT 1`, p.org, p.hash).Scan(&blob, &mime, &bytesN, &ms, &timings)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET audio_status='ready', audio_hash=$2, audio_blob=$3, audio_mime=$4, audio_bytes=$5, audio_ms=$6,
		audio_timings=$7, audio_error='', audio_at=now() WHERE id=$1::uuid`, p.epID, p.hash, blob, mime, bytesN, ms, timings)
	return err == nil, err
}

// ---- worker -----------------------------------------------------------------------

// Start runs the audio worker and the publishing scheduler until Stop.
func (s *Stories) Start() {
	if s.Workers <= 0 {
		s.Workers = 2
	}
	ctx := context.Background()
	// A restart interrupts running jobs; put them back in the queue.
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_audio_jobs SET status='queued', progress=0 WHERE status='running'`)
	for i := 0; i < s.Workers; i++ {
		go s.workLoop()
	}
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		time.Sleep(3 * time.Second)
		for {
			s.publishDue(ctx)
			select {
			case <-t.C:
			case <-s.stop:
				return
			}
		}
	}()
}

// Stop ends the background loops.
func (s *Stories) Stop() { s.stopOnce.Do(func() { close(s.stop) }) }

func (s *Stories) workLoop() {
	ctx := context.Background()
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		for s.runOne(ctx) {
		}
		select {
		case <-s.kick:
		case <-t.C:
		case <-s.stop:
			return
		}
	}
}

// RunPending claims and runs one queued job; it reports whether there was one.
// The worker calls it in a loop; tests call it directly.
func (s *Stories) RunPending(ctx context.Context) bool { return s.runOne(ctx) }

func (s *Stories) runOne(ctx context.Context) bool {
	var jobID, org, epID, user string
	err := s.Pool.QueryRow(ctx, `UPDATE adrient.story_audio_jobs SET status='running', started_at=now(), progress=0
		WHERE id=(SELECT id FROM adrient.story_audio_jobs WHERE status='queued' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id::text, org_id::text, episode_id::text, user_id::text`).Scan(&jobID, &org, &epID, &user)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log("claim job", err)
			time.Sleep(time.Second)
		}
		return false
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET audio_status='generating' WHERE id=$1::uuid AND audio_blob=''`, epID)
	jctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err := s.generate(jctx, jobID, org, epID, user); err != nil {
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		s.log("audio job failed", err)
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_audio_jobs SET status='failed', error=$2, finished_at=now() WHERE id=$1::uuid`, jobID, msg)
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET audio_error=$2, audio_status=CASE WHEN audio_blob<>'' THEN 'ready' ELSE 'failed' END WHERE id=$1::uuid`, epID, msg)
		if s.Audit != nil {
			s.Audit(ctx, org, user, "story.audio_failed", epID, map[string]any{"error": msg})
		}
		return true
	}
	return true
}

func (s *Stories) generate(ctx context.Context, jobID, org, epID, user string) error {
	p, err := s.loadPlan(ctx, org, epID)
	if err != nil {
		return err
	}
	if p.prov == nil {
		return errors.New("no server voice is set up any more")
	}
	if len(p.sents) == 0 {
		return errors.New("the story has no text to read")
	}
	if ok, err := s.reuseCached(ctx, p); err == nil && ok {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_audio_jobs SET status='done', progress=100, finished_at=now() WHERE id=$1::uuid`, jobID)
		return nil
	}
	prov, err := s.build(org, *p.prov)
	if err != nil {
		return err
	}
	voices := map[string]string{"": p.eff.Voice}
	for k, v := range p.eff.VoiceMap {
		if v != "" {
			voices[k] = v
		}
	}
	last := time.Now()
	syn := &tts.Synthesizer{Provider: prov, Voices: voices, Language: p.eff.Language, Speed: p.eff.Speed, Pitch: p.eff.Pitch, MaxChars: 3000,
		Progress: func(done, total int) {
			if time.Since(last) > 500*time.Millisecond || done == total {
				last = time.Now()
				_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_audio_jobs SET progress=$2 WHERE id=$1::uuid`, jobID, done*100/total)
			}
		}}
	out, err := syn.Run(ctx, p.sents)
	if err != nil {
		return err
	}
	if s.Blobs == nil {
		return errors.New("file storage is not available")
	}
	ext := ".wav"
	if out.Mime == tts.MimeMP3 {
		ext = ".mp3"
	}
	key := "stories/audio/" + org + "/" + p.hash + ext
	if err := s.Blobs.Put(ctx, key, out.Mime, int64(len(out.Audio)), bytes.NewReader(out.Audio)); err != nil {
		return fmt.Errorf("saving audio: %w", err)
	}
	timings, _ := json.Marshal(out.Timings)
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.story_episodes SET audio_status='ready', audio_hash=$2, audio_blob=$3, audio_mime=$4, audio_bytes=$5, audio_ms=$6,
		audio_timings=$7, audio_error='', audio_at=now() WHERE id=$1::uuid`, epID, p.hash, key, out.Mime, len(out.Audio), out.DurationMs, timings)
	if err != nil {
		return err
	}
	old := p.audioBlob
	if tag.RowsAffected() == 0 {
		s.dropBlobs(ctx, org, []string{key})
	} else if old != "" && old != key {
		s.dropBlobs(ctx, org, []string{old})
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_audio_jobs SET status='done', progress=100, finished_at=now() WHERE id=$1::uuid`, jobID)
	if s.Audit != nil {
		s.Audit(ctx, org, user, "story.audio_ready", epID, map[string]any{"duration_ms": out.DurationMs, "bytes": len(out.Audio), "provider": p.prov.Name})
	}
	return nil
}

// AudioFile is a stored narration ready to stream.
type AudioFile struct {
	Data     []byte
	Mime     string
	Name     string
	Hash     string
	Modified time.Time
}

// OpenAudio loads the audio of an episode the viewer may listen to. Download
// additionally needs the show to allow it (or the viewer to be its author).
func (s *Stories) OpenAudio(ctx context.Context, v Viewer, epID string, download, mayDownload bool) (AudioFile, error) {
	var af AudioFile
	if !isUUID(epID) || s.Blobs == nil {
		return af, ErrNotFound
	}
	var blob, title string
	var author string
	var allow bool
	err := s.Pool.QueryRow(ctx, `SELECT e.audio_blob, e.audio_mime, e.audio_hash, e.audio_at, e.title, s.author_id::text, s.allow_download
		FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id
		WHERE e.id=$5::uuid AND e.audio_status='ready' AND e.audio_blob<>'' AND (e.status='published' OR s.author_id=$2::uuid) AND `+visible, v.args(epID)...).
		Scan(&blob, &af.Mime, &af.Hash, &af.Modified, &title, &author, &allow)
	if errors.Is(err, pgx.ErrNoRows) {
		return af, ErrNotFound
	}
	if err != nil {
		return af, err
	}
	if download && author != v.UserID && !(allow && mayDownload) {
		return af, forbidden("downloading is not allowed for this story")
	}
	rc, _, size, err := s.Blobs.Get(ctx, blob)
	if err != nil {
		return af, ErrNotFound
	}
	defer rc.Close()
	if size > 200<<20 {
		return af, invalid("audio file is too large to stream")
	}
	af.Data, err = io.ReadAll(rc)
	if err != nil {
		return af, err
	}
	ext := ".wav"
	if af.Mime == tts.MimeMP3 {
		ext = ".mp3"
	}
	af.Name = safeFileName(title) + ext
	return af, nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func safeFileName(s string) string {
	s = unsafeName.ReplaceAllString(s, "")
	if s == "" {
		s = "story"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// Jobs lists recent audio jobs of the org (admin view).
type JobRow struct {
	ID        string     `json:"id"`
	Episode   string     `json:"episode"`
	EpisodeID string     `json:"episode_id"`
	User      string     `json:"user"`
	Status    string     `json:"status"`
	Progress  int        `json:"progress"`
	Chars     int        `json:"chars"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Finished  *time.Time `json:"finished_at,omitempty"`
}

// ListJobs returns the 50 newest audio jobs.
func (s *Stories) ListJobs(ctx context.Context, org string) ([]JobRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT j.id::text, e.title, e.id::text, u.display_name, j.status, j.progress, j.chars, j.error, j.created_at, j.finished_at
		FROM adrient.story_audio_jobs j JOIN adrient.story_episodes e ON e.id=j.episode_id JOIN adrient.users u ON u.id=j.user_id
		WHERE j.org_id=$1::uuid ORDER BY j.created_at DESC LIMIT 50`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobRow{}
	for rows.Next() {
		var j JobRow
		if err := rows.Scan(&j.ID, &j.Episode, &j.EpisodeID, &j.User, &j.Status, &j.Progress, &j.Chars, &j.Error, &j.CreatedAt, &j.Finished); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
