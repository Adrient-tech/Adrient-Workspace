package podcasts

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage/storagetest"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tts"
)

type memBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBlobs) Put(_ context.Context, key, _ string, _ int64, body io.Reader) error {
	raw, _ := io.ReadAll(body)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[string][]byte{}
	}
	b.m[key] = raw
	return nil
}
func (b *memBlobs) Get(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	raw, ok := b.m[key]
	if !ok {
		return nil, "", 0, fmt.Errorf("no such blob")
	}
	return io.NopCloser(bytes.NewReader(raw)), "", int64(len(raw)), nil
}
func (b *memBlobs) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.m, key)
	return nil
}
func (b *memBlobs) count() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.m) }

// fakeSpeech is an OpenAI-compatible speech server (about 100 ms of tone per character).
type fakeSpeech struct {
	mu   sync.Mutex
	hits int
	last map[string]any
	fail bool
}

func (f *fakeSpeech) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/audio/voices":
		_ = json.NewEncoder(w).Encode(map[string]any{"voices": []string{"af_bella", "hi_priya", "ta_kavya"}})
	case "/v1/audio/speech":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.mu.Lock()
		f.hits++
		f.last = m
		fail := f.fail
		f.mu.Unlock()
		if fail {
			http.Error(w, "overloaded", 503)
			return
		}
		n := len(m["input"].(string)) * 80 // samples at 8 kHz = 10 ms per character
		pcm := make([]byte, n*2)
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(6000*math.Sin(float64(i)/9))))
		}
		_, _ = w.Write(tts.MakeWAV(8000, pcm))
	default:
		http.NotFound(w, r)
	}
}

type env struct {
	t      *testing.T
	s      *Stories
	pool   *pgxpool.Pool
	org    string
	alice  Viewer // author
	bob    Viewer // listener
	blobs  *memBlobs
	speech *fakeSpeech
	srv    *httptest.Server
	mu     sync.Mutex
	notes  []string
}

func newEnv(t *testing.T) *env {
	pool := storagetest.ScratchDB(t)
	ctx := context.Background()
	e := &env{t: t, pool: pool, blobs: &memBlobs{}, speech: &fakeSpeech{}}
	e.srv = httptest.NewServer(e.speech)
	t.Cleanup(e.srv.Close)
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('stories-test','Stories test') RETURNING id::text`).Scan(&e.org); err != nil {
		t.Fatal(err)
	}
	mk := func(name string) Viewer {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'t',$2,$3,$4) RETURNING id::text`,
			e.org, name, name+"@example.test", name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return Viewer{OrgID: e.org, UserID: id, Groups: []string{}, Depts: []string{}}
	}
	e.alice, e.bob = mk("Alice"), mk("Bob")
	e.s = NewStories(pool, e.blobs)
	e.s.Secret = "test-secret"
	e.s.Notify = func(_ context.Context, org, user, actor, typ, title, body, url string) {
		e.mu.Lock()
		e.notes = append(e.notes, user+"|"+typ+"|"+title+"|"+url)
		e.mu.Unlock()
	}
	return e
}

func (e *env) provider(kind string) ProviderRow {
	var in ProviderInput
	switch kind {
	case "openai":
		in = ProviderInput{Name: "Local speech", Kind: "openai", BaseURL: e.srv.URL + "/v1", APIKey: "sk-test", Model: "kokoro", Format: "wav", DefaultVoice: "af_bella"}
	case "command":
		in = ProviderInput{Name: "Fake command", Kind: "command", Command: os.Args[0], Args: []string{"-test.run=^TestHelperProcess$", "--", "{voice}", "{out}"},
			Voices: []tts.Voice{{ID: "en-us", Name: "English"}}, DefaultVoice: "en-us"}
	}
	t := true
	in.IsDefault = &t
	p, err := e.s.SaveProvider(context.Background(), e.org, "", in)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// TestHelperProcess is the fake command-line voice: it reads the text and writes a WAV file.
func TestHelperProcess(t *testing.T) {
	for i, a := range os.Args {
		if a != "--" {
			continue
		}
		rest := os.Args[i+1:]
		buf := make([]byte, 1<<16)
		n, _ := os.Stdin.Read(buf)
		pcm := make([]byte, n*800*2)
		_ = os.WriteFile(rest[1], tts.MakeWAV(8000, pcm), 0o600)
		os.Exit(0)
	}
}

const storyHTML = `<h2>Day one</h2><p>Welcome to Adrient.</p><p>[Meera] Hello there, new friend. How was the trip?</p><p>[Narrator] She smiled and walked in.</p><script>alert(1)</script>`

func ptr[T any](v T) *T { return &v }

func (e *env) publishedEpisode(v Viewer, title string) (Show, EpisodeDetail) {
	ctx := context.Background()
	sh, err := e.s.CreateShow(ctx, v, ShowInput{Title: &title, Genre: ptr("Onboarding")})
	if err != nil {
		e.t.Fatal(err)
	}
	ep, err := e.s.CreateEpisode(ctx, v, sh.ID, EpisodeInput{Title: ptr("Episode 1"), BodyHTML: ptr(storyHTML)})
	if err != nil {
		e.t.Fatal(err)
	}
	return sh, ep
}

func TestStoryLifecycleAndAudio(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.provider("openai")
	sh, ep := e.publishedEpisode(e.alice, "Welcome stories")
	if strings.Contains(ep.BodyHTML, "script") || strings.Contains(ep.BodyHTML, "alert") {
		t.Fatalf("html not sanitised: %s", ep.BodyHTML)
	}
	if ep.Effective.Mode != "server" || len(ep.Sentences) < 4 || ep.Sentences[2].Speaker != "Meera" {
		t.Fatalf("effective %+v sentences %+v", ep.Effective, ep.Sentences)
	}
	if _, err := e.s.GetEpisode(ctx, e.bob, ep.ID); err != ErrNotFound {
		t.Fatalf("draft visible to bob: %v", err)
	}
	if l, _ := e.s.ListShows(ctx, e.bob, ShowFilter{}); len(l) != 0 {
		t.Fatal("show without published episode listed")
	}
	if _, err := e.s.Publish(ctx, e.bob, ep.ID, nil); err == nil {
		t.Fatal("bob published alice's story")
	}
	if err := e.s.SetFollow(ctx, e.bob, "author", e.alice.UserID, true); err != nil {
		t.Fatal(err)
	}
	pub, err := e.s.Publish(ctx, e.alice, ep.ID, nil)
	if err != nil || pub.Status != "published" {
		t.Fatalf("publish: %v %+v", err, pub.Status)
	}
	time.Sleep(300 * time.Millisecond) // the notification is sent in the background
	e.mu.Lock()
	notes := append([]string{}, e.notes...)
	e.mu.Unlock()
	if len(notes) != 1 || !strings.Contains(notes[0], "story_episode") || !strings.HasPrefix(notes[0], e.bob.UserID) {
		t.Fatalf("notifications %v", notes)
	}
	d, _ := e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.AudioStatus != "queued" || d.Job == nil || d.Job.Status != "queued" {
		t.Fatalf("expected queued job, got %s %+v", d.AudioStatus, d.Job)
	}
	if e.speech.hits != 0 {
		t.Fatal("audio generated on the request thread")
	}
	if !e.s.RunPending(ctx) {
		t.Fatal("no job to run")
	}
	d, _ = e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.AudioStatus != "ready" || !d.HasAudio || d.DurationMs < 500 || len(d.Timings) != len(d.Sentences) || d.AudioStale {
		t.Fatalf("after job: %s dur=%d timings=%d stale=%v err=%s", d.AudioStatus, d.DurationMs, len(d.Timings), d.AudioStale, d.AudioError)
	}
	if e.speech.last["voice"] == nil || e.speech.last["model"] != "kokoro" {
		t.Fatalf("speech request %v", e.speech.last)
	}
	if d.Timings[len(d.Timings)-1].End < d.DurationMs-20 {
		t.Fatalf("timings do not cover the audio: %+v dur %d", d.Timings, d.DurationMs)
	}
	af, err := e.s.OpenAudio(ctx, e.bob, ep.ID, false, true)
	if err != nil || af.Mime != tts.MimeWAV || len(af.Data) < 1000 {
		t.Fatalf("bob audio: %v", err)
	}
	if _, err := e.s.OpenAudio(ctx, e.bob, ep.ID, true, true); err == nil {
		t.Fatal("download allowed although the author did not allow it")
	}
	if _, err := e.s.UpdateShow(ctx, e.alice, sh.ID, ShowInput{AllowDownload: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.OpenAudio(ctx, e.bob, ep.ID, true, true); err != nil {
		t.Fatalf("download after allow: %v", err)
	}
	if _, err := e.s.OpenAudio(ctx, e.bob, ep.ID, true, false); err == nil {
		t.Fatal("download allowed although the org control denies it")
	}
	hits := e.speech.hits
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, false); err != nil {
		t.Fatal(err)
	}
	if e.s.RunPending(ctx) || e.speech.hits != hits {
		t.Fatal("unchanged text regenerated")
	}
	ep2, _ := e.s.CreateEpisode(ctx, e.alice, sh.ID, EpisodeInput{Title: ptr("Episode 2"), BodyHTML: ptr(storyHTML)})
	if _, err := e.s.RequestAudio(ctx, e.alice, ep2.ID, false); err != nil {
		t.Fatal(err)
	}
	d2, _ := e.s.GetEpisode(ctx, e.alice, ep2.ID)
	if d2.AudioStatus != "ready" || e.speech.hits != hits || e.blobs.count() != 1 {
		t.Fatalf("cache miss: %s hits %d->%d blobs %d", d2.AudioStatus, hits, e.speech.hits, e.blobs.count())
	}
	upd, err := e.s.UpdateEpisode(ctx, e.alice, ep.ID, EpisodeInput{BodyHTML: ptr("<p>A completely different story now.</p>")})
	if err != nil {
		t.Fatal(err)
	}
	if !upd.AudioStale || upd.Job == nil || upd.Job.Status != "queued" {
		t.Fatalf("stale=%v job=%+v", upd.AudioStale, upd.Job)
	}
	e.s.RunPending(ctx)
	upd, _ = e.s.GetEpisode(ctx, e.alice, ep.ID)
	if upd.AudioStale || upd.AudioStatus != "ready" {
		t.Fatalf("not regenerated: %+v", upd.AudioStatus)
	}
	if _, err := e.s.Unpublish(ctx, e.alice, ep.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.OpenAudio(ctx, e.bob, ep.ID, false, true); err != ErrNotFound {
		t.Fatalf("unpublished audio still open: %v", err)
	}
	if err := e.s.DeleteShow(ctx, e.alice, sh.ID); err != nil {
		t.Fatal(err)
	}
	if e.blobs.count() != 0 {
		t.Fatalf("blobs left: %d", e.blobs.count())
	}
}

func TestGenerationFailureAndLimits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.provider("openai")
	_, ep := e.publishedEpisode(e.alice, "Failing")
	e.speech.fail = true
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, false); err != nil {
		t.Fatal(err)
	}
	e.s.RunPending(ctx)
	d, _ := e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.AudioStatus != "failed" || !strings.Contains(d.AudioError, "503") || d.Job.Status != "failed" {
		t.Fatalf("failure not reported: %s %q %+v", d.AudioStatus, d.AudioError, d.Job)
	}
	if _, err := e.s.SaveSettings(ctx, e.org, Settings{DailyChars: 10, PerMinuteJobs: 6, MaxChars: 60000, AutoGenerate: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, true); err == nil || !strings.Contains(err.Error(), "daily audio limit") {
		t.Fatalf("quota not enforced: %v", err)
	}
	e.speech.fail = false
	if _, err := e.s.SaveSettings(ctx, e.org, Settings{DailyChars: 1000000, PerMinuteJobs: 1, MaxChars: 60000, AutoGenerate: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, true); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("rate limit not enforced: %v", err)
	}
	if _, err := e.s.SaveSettings(ctx, e.org, Settings{DailyChars: 1000000, PerMinuteJobs: 60, MaxChars: 500, AutoGenerate: false}); err != nil {
		t.Fatal(err)
	}
	long := "<p>" + strings.Repeat("Another sentence goes here. ", 40) + "</p>"
	if _, err := e.s.UpdateEpisode(ctx, e.alice, ep.ID, EpisodeInput{BodyHTML: &long}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, true); err == nil || !strings.Contains(err.Error(), "limit for audio") {
		t.Fatalf("max chars not enforced: %v", err)
	}
	e2 := newEnv(t)
	_, ep2 := e2.publishedEpisode(e2.alice, "Browser only")
	if ep2.Effective.Mode != "browser" {
		t.Fatalf("mode %s", ep2.Effective.Mode)
	}
	if _, err := e2.s.RequestAudio(ctx, e2.alice, ep2.ID, false); err == nil || !strings.Contains(err.Error(), "browser") {
		t.Fatalf("expected browser hint, got %v", err)
	}
	if pub, err := e2.s.Publish(ctx, e2.alice, ep2.ID, nil); err != nil || pub.Status != "published" {
		t.Fatalf("publish without voices: %v", err)
	}
}

func TestCommandProviderEndToEnd(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.provider("command")
	_, ep := e.publishedEpisode(e.alice, "Command voice")
	if _, err := e.s.RequestAudio(ctx, e.alice, ep.ID, false); err != nil {
		t.Fatal(err)
	}
	e.s.RunPending(ctx)
	d, _ := e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.AudioStatus != "ready" || d.DurationMs < 500 {
		t.Fatalf("%s %q %d", d.AudioStatus, d.AudioError, d.DurationMs)
	}
}

func TestAudienceScheduleAndSocial(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var group string
	if err := e.pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, name, member_ids) VALUES ($1,'Safety team','[]'::jsonb) RETURNING id::text`, e.org).Scan(&group); err != nil {
		t.Skipf("cannot create a group: %v", err)
	}
	sh, ep := e.publishedEpisode(e.alice, "Safety series")
	if _, err := e.s.UpdateShow(ctx, e.alice, sh.ID, ShowInput{Audience: ptr("group"), AudienceID: &group}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(2 * time.Hour)
	if _, err := e.s.Publish(ctx, e.alice, ep.ID, &at); err != nil {
		t.Fatal(err)
	}
	d, _ := e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.Status != "scheduled" {
		t.Fatalf("status %s", d.Status)
	}
	e.s.Now = func() time.Time { return time.Now().Add(3 * time.Hour) }
	e.s.publishDue(ctx)
	e.s.Now = nil
	d, _ = e.s.GetEpisode(ctx, e.alice, ep.ID)
	if d.Status != "published" {
		t.Fatalf("not published by scheduler: %s", d.Status)
	}
	if _, err := e.s.GetShow(ctx, e.bob, sh.ID); err != ErrNotFound {
		t.Fatalf("bob sees group-only show: %v", err)
	}
	if h, _ := e.s.Home(ctx, e.bob); len(h.New) != 0 || len(h.Trending) != 0 {
		t.Fatal("group-only story on bob's home")
	}
	if r, _ := e.s.Search(ctx, e.bob, "safety"); len(r.Shows) != 0 || len(r.Episodes) != 0 {
		t.Fatal("group-only story in bob's search")
	}
	if _, err := e.pool.Exec(ctx, `UPDATE adrient.groups SET member_ids=$2::jsonb WHERE id=$1::uuid`, group, fmt.Sprintf(`["%s"]`, e.bob.UserID)); err != nil {
		t.Fatal(err)
	}
	bob, _ := e.s.LoadViewer(ctx, e.org, e.bob.UserID, false)
	if _, err := e.s.GetEpisode(ctx, bob, ep.ID); err != nil {
		t.Fatalf("group member cannot open: %v", err)
	}
	if err := e.s.RecordPlay(ctx, bob, ep.ID); err != nil {
		t.Fatal(err)
	}
	_ = e.s.RecordPlay(ctx, bob, ep.ID)
	if err := e.s.SaveProgress(ctx, bob, ep.ID, Progress{PositionMs: 4000, DurationMs: 20000}); err != nil {
		t.Fatal(err)
	}
	h, _ := e.s.Home(ctx, bob)
	if len(h.Continue) != 1 || h.Continue[0].PositionMs != 4000 {
		t.Fatalf("continue shelf %+v", h.Continue)
	}
	_ = e.s.SaveProgress(ctx, bob, ep.ID, Progress{PositionMs: 19500, DurationMs: 20000})
	if h, _ := e.s.Home(ctx, bob); len(h.Continue) != 0 {
		t.Fatal("finished episode still in continue listening")
	}
	_ = e.s.MarkPlayed(ctx, bob, ep.ID, false)
	d, _ = e.s.GetEpisode(ctx, bob, ep.ID)
	if d.Completed || d.PositionMs != 0 {
		t.Fatalf("unplayed %+v", d.Episode)
	}
	_ = e.s.SetLike(ctx, bob, ep.ID, true)
	_ = e.s.SetLibrary(ctx, bob, "episode", ep.ID, true)
	_ = e.s.SetLibrary(ctx, bob, "show", sh.ID, true)
	lib, _ := e.s.MyLibrary(ctx, bob)
	if len(lib.Episodes) != 1 || len(lib.Shows) != 1 || len(lib.Liked) != 1 {
		t.Fatalf("library %+v", lib)
	}
	c, err := e.s.AddComment(ctx, bob, ep.ID, "Lovely story")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.ReportComment(ctx, e.alice, c.ID, "off topic"); err != nil {
		t.Fatal(err)
	}
	q, _ := e.s.ReportedComments(ctx, e.alice)
	if len(q) != 1 {
		t.Fatalf("moderation queue %v", q)
	}
	if err := e.s.ModerateComment(ctx, bob, c.ID, "hide"); err == nil {
		t.Fatal("a listener hid a comment")
	}
	if err := e.s.ModerateComment(ctx, e.alice, c.ID, "hide"); err != nil {
		t.Fatal(err)
	}
	if l, _ := e.s.ListComments(ctx, bob, ep.ID); len(l) != 1 || l[0].Status != "hidden" {
		t.Fatalf("comments %+v", l)
	}
	_, _ = e.s.UpdateShow(ctx, e.alice, sh.ID, ShowInput{CommentsOn: ptr(false)})
	if _, err := e.s.AddComment(ctx, bob, ep.ID, "again"); err == nil {
		t.Fatal("comment accepted although comments are off")
	}
	st, err := e.s.Stats(ctx, e.alice, sh.ID)
	if err != nil || st.Plays != 1 || st.Likes != 1 || len(st.Episodes) != 1 || len(st.Daily) != 14 {
		t.Fatalf("stats %+v %v", st, err)
	}
	if _, err := e.s.Stats(ctx, bob, sh.ID); err == nil {
		t.Fatal("listener read author stats")
	}
}

func TestProviderKeysAreWriteOnly(t *testing.T) {
	e := newEnv(t)
	p := e.provider("openai")
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "sk-test") || p.KeySource != "stored" {
		t.Fatalf("key leaked or not stored: %s", raw)
	}
	var sealed []byte
	_ = e.pool.QueryRow(context.Background(), `SELECT key_sealed FROM adrient.story_voice_providers WHERE id=$1::uuid`, p.ID).Scan(&sealed)
	if bytes.Contains(sealed, []byte("sk-test")) || len(sealed) < 20 {
		t.Fatal("key not encrypted")
	}
	vl, err := e.s.Voices(context.Background(), e.org)
	if err != nil || len(vl) != 1 || len(vl[0].Voices) != 3 {
		t.Fatalf("voices %+v %v", vl, err)
	}
	if r := e.s.TestProvider(context.Background(), e.org, p.ID, "af_bella"); !r.OK || r.Voices != 3 {
		t.Fatalf("test provider %+v", r)
	}
	e.s.Secret = ""
	if _, err := e.s.SaveProvider(context.Background(), e.org, "", ProviderInput{Name: "x", Kind: "openai", BaseURL: e.srv.URL + "/v1", APIKey: "k"}); err == nil {
		t.Fatal("key stored without a secret")
	}
}
