package demoseed

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/music"
)

// v6Media seeds Music and Video content. The media is generated with
// ffmpeg (sine-based melodies, test-pattern clips, short captions), so no
// third-party audio or footage is involved. Without ffmpeg or a blob store the
// step is skipped with a note.
func (s *seeder) v6Media() {
	if s.opt.Blobs == nil {
		s.rep.Skipped = append(s.rep.Skipped, "Music and Video: not seeded (needs a blob store)")
		return
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		s.rep.Skipped = append(s.rep.Skipped, "Music and Video: not seeded (ffmpeg not installed)")
		return
	}
	dir, err := os.MkdirTemp("", "mvseed-")
	if err != nil {
		s.warn("media temp dir", err)
		return
	}
	defer os.RemoveAll(dir)
	people := s.mediaPeople()
	s.seedMusic(ff, dir, people)
	s.seedVideos(ff, dir, people)
}

// mediaPeople returns the admin first, then up to nine other users of the org.
func (s *seeder) mediaPeople() []string {
	out := []string{s.admin}
	rows, err := s.pool.Query(s.ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1::uuid AND id<>$2::uuid ORDER BY email LIMIT 9`, s.org, s.admin)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

func runFF(ctx context.Context, ff string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, ff, append([]string{"-y", "-v", "error"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func dataURL(mime string, b []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func randKey(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "/demo-" + hex.EncodeToString(b)
}

// ---- Music -----------------------------------------------------------------

type demoTrack struct {
	title, artist, album, genre string
	no, year                    int
	root                        float64 // Hz
	scale                       []int   // semitones
	bpm                         float64
	hue0, hue1                  string
}

// melody builds an ffmpeg aevalsrc expression: a looping arpeggio with a soft
// decay plus a low pad.
func melody(t demoTrack) string {
	step := 60.0 / t.bpm / 2
	n := len(t.scale)
	var sel strings.Builder
	for i := n - 1; i >= 0; i-- {
		f := t.root * math.Pow(2, float64(t.scale[i])/12)
		if i == n-1 {
			fmt.Fprintf(&sel, "%.3f", f)
		} else {
			s := sel.String()
			sel.Reset()
			fmt.Fprintf(&sel, "if(eq(mod(floor(t/%.4f),%d),%d),%.3f,%s)", step, n, i, f, s)
		}
	}
	lead := fmt.Sprintf("0.28*sin(2*PI*(%s)*t)*exp(-3.5*mod(t,%.4f))", sel.String(), step)
	pad := fmt.Sprintf("0.10*sin(2*PI*%.3f*t)+0.05*sin(2*PI*%.3f*t)", t.root/2, t.root*0.75)
	return lead + "+" + pad
}

var demoTracks = []demoTrack{
	{"Morning Ferry", "Lumen Harbor", "Slow Tide", "Ambient", 1, 2024, 220, []int{0, 4, 7, 12, 7, 4}, 84, "0x126C5A", "0x7fd1b9"},
	{"Salt Air", "Lumen Harbor", "Slow Tide", "Ambient", 2, 2024, 196, []int{0, 3, 7, 10, 7, 3}, 72, "0x126C5A", "0x7fd1b9"},
	{"Low Light", "Lumen Harbor", "Slow Tide", "Ambient", 3, 2024, 174.6, []int{0, 5, 9, 12, 9, 5}, 66, "0x126C5A", "0x7fd1b9"},
	{"Paper Lanterns", "Quiet Orchard", "Window Seat", "Folk", 1, 2023, 246.9, []int{0, 2, 4, 7, 9, 7, 4, 2}, 108, "0xF2A21B", "0xfbe3a8"},
	{"Orchard Road", "Quiet Orchard", "Window Seat", "Folk", 2, 2023, 261.6, []int{0, 4, 7, 9, 7, 4}, 96, "0xF2A21B", "0xfbe3a8"},
	{"Kettle Song", "Quiet Orchard", "Window Seat", "Folk", 3, 2023, 293.7, []int{0, 2, 7, 9, 7, 2}, 120, "0xF2A21B", "0xfbe3a8"},
	{"Night Bus", "Sable Trio", "After Hours", "Jazz", 1, 2025, 207.7, []int{0, 3, 7, 10, 14, 10, 7, 3}, 132, "0x3b4a6b", "0xaab7d6"},
	{"Corner Table", "Sable Trio", "After Hours", "Jazz", 2, 2025, 233.1, []int{0, 3, 5, 7, 10, 7, 5, 3}, 100, "0x3b4a6b", "0xaab7d6"},
	{"Last Tram", "Sable Trio", "After Hours", "Jazz", 3, 2025, 185, []int{0, 4, 7, 11, 7, 4}, 88, "0x3b4a6b", "0xaab7d6"},
	{"Open Field", "Quiet Orchard", "Open Field", "Folk", 1, 2025, 329.6, []int{0, 4, 7, 12, 16, 12, 7, 4}, 112, "0xc2563a", "0xf0b9a6"},
	{"Harbor Lights", "Lumen Harbor", "Glass Hours", "Electronic", 1, 2025, 261.6, []int{0, 7, 12, 7, 15, 7}, 124, "0x6b3fa0", "0xcdb4ea"},
	{"Static Bloom", "Lumen Harbor", "Glass Hours", "Electronic", 2, 2025, 311.1, []int{0, 5, 7, 12, 7, 5}, 128, "0x6b3fa0", "0xcdb4ea"},
}

func lrcFor(title string, dur int) string {
	lines := []string{"(" + title + ")", "Small boats drifting out", "Light on the water, light on the wall", "Nobody here to count the hours",
		"Leave the window open", "Let the evening in", "We were never in a hurry", "Hold the quiet a little longer", "Say it softly, say it again", "Morning finds us anyway"}
	var b strings.Builder
	fmt.Fprintf(&b, "[ti:%s]\n[ar:Demo artist]\n", title)
	gap := float64(dur-4) / float64(len(lines))
	for i, l := range lines {
		t := 2 + gap*float64(i)
		fmt.Fprintf(&b, "[%02d:%05.2f]%s\n", int(t)/60, math.Mod(t, 60), l)
	}
	return b.String()
}

func (s *seeder) seedMusic(ff, dir string, people []string) {
	const dur = 48
	var ids []string
	cover := map[string]string{}
	for i, t := range demoTracks {
		key := t.album
		cp, ok := cover[key]
		if !ok {
			cp = filepath.Join(dir, fmt.Sprintf("cover%d.jpg", len(cover)))
			if err := runFF(s.ctx, ff, "-f", "lavfi", "-i", fmt.Sprintf("gradients=s=500x500:c0=%s:c1=%s:x0=0:y0=0:x1=500:y1=500:d=1:seed=%d", t.hue0, t.hue1, i+3),
				"-frames:v", "1", "-q:v", "5", cp); err != nil {
				s.warn("music cover", err)
				return
			}
			cover[key] = cp
		}
		mp3 := filepath.Join(dir, fmt.Sprintf("t%02d.mp3", i))
		err := runFF(s.ctx, ff, "-f", "lavfi", "-i", fmt.Sprintf("aevalsrc='%s':s=44100:d=%d", melody(t), dur), "-i", cp,
			"-map", "0:a", "-map", "1:v", "-c:a", "libmp3lame", "-b:a", "96k", "-c:v", "mjpeg", "-id3v2_version", "3",
			"-metadata", "title="+t.title, "-metadata", "artist="+t.artist, "-metadata", "album="+t.album, "-metadata", "genre="+t.genre,
			"-metadata", fmt.Sprintf("track=%d", t.no), "-metadata", fmt.Sprintf("date=%d", t.year),
			"-metadata:s:v", "title=Cover", "-metadata:s:v", "comment=Cover (front)", mp3)
		if err != nil {
			s.warn("music track", err)
			return
		}
		data, _ := os.ReadFile(mp3)
		tags := music.ReadTags(bytes.NewReader(data))
		blob := randKey("music")
		if err := s.opt.Blobs.Put(s.ctx, blob, "audio/mpeg", int64(len(data)), bytes.NewReader(data)); err != nil {
			s.warn("music blob", err)
			return
		}
		lyrics := ""
		if i%2 == 0 || i < 6 {
			lyrics = lrcFor(t.title, dur)
		}
		owner := people[i%min(len(people), 4)]
		var id string
		err = s.pool.QueryRow(s.ctx, `INSERT INTO adrient.music_tracks (org_id, owner_id, title, artist, album, genre, track_no, year, content_type, size,
			duration_seconds, artwork_data_url, blob_key, lyrics, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'audio/mpeg',$9,$10,$11,$12,$13, now() - ($14 || ' hours')::interval) RETURNING id::text`,
			s.org, owner, firstNonEmptyStr(tags.Title, t.title), firstNonEmptyStr(tags.Artist, t.artist), firstNonEmptyStr(tags.Album, t.album),
			firstNonEmptyStr(tags.Genre, t.genre), max(tags.TrackNo, t.no), max(tags.Year, t.year), len(data), float64(dur), tags.ArtDataURL(), blob, lyrics,
			fmt.Sprint((len(demoTracks)-i)*7)).Scan(&id)
		if err != nil {
			s.warn("music insert", err)
			return
		}
		ids = append(ids, id)
		s.count("music tracks", 1)
		if i == 3 {
			// One track also goes into Drive, with its .lrc next to it, to show the Drive import.
			s.driveMusic(data, t, lrcFor(t.title, dur))
		}
	}
	// Likes, plays and playlists for the admin and a few colleagues.
	for i, id := range ids {
		if i%3 == 0 {
			s.exec("music like", `INSERT INTO adrient.music_likes (user_id, track_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, s.admin, id)
		}
	}
	plays := []int{0, 3, 6, 0, 9, 3, 10, 1, 6, 6, 4, 0, 7, 3, 3, 11, 2, 6}
	for k, ti := range plays {
		who := people[k%min(len(people), 3)]
		if k%3 != 2 {
			who = s.admin
		}
		s.exec("music play", `INSERT INTO adrient.music_plays (org_id, user_id, track_id, seconds_listened, played_at)
			VALUES ($1,$2,$3,$4, now() - ($5 || ' minutes')::interval)`, s.org, who, ids[ti], 40.0, fmt.Sprint(k*95+12))
	}
	gid := s.demoGroup(people)
	type pl struct {
		name, desc, vis string
		idx             []int
	}
	for _, p := range []pl{
		{"Focus hours", "Instrumental pieces for deep work", "org", []int{0, 1, 2, 10, 11}},
		{"Evening wind-down", "Slow and warm", "private", []int{6, 7, 8, 4}},
		{"Studio crew picks", "What the design team is playing", "group", []int{3, 4, 5, 9, 6}},
	} {
		var pid string
		var g any
		if p.vis == "group" && gid != "" {
			g = gid
		}
		if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.music_playlists (org_id, owner_id, name, description, visibility, group_id)
			VALUES ($1,$2,$3,$4,$5,$6::uuid) RETURNING id::text`, s.org, s.admin, p.name, p.desc, p.vis, g).Scan(&pid); err != nil {
			s.warn("music playlist", err)
			continue
		}
		for pos, ti := range p.idx {
			s.exec("music playlist track", `INSERT INTO adrient.music_playlist_tracks (playlist_id, track_id, position) VALUES ($1,$2,$3)`, pid, ids[ti], pos)
		}
		s.count("music playlists", 1)
	}
}

func (s *seeder) exec(what, q string, args ...any) {
	if _, err := s.pool.Exec(s.ctx, q, args...); err != nil {
		s.warn(what, err)
	}
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// demoGroup creates (once) a group the admin and a few colleagues belong to.
func (s *seeder) demoGroup(people []string) string {
	var id string
	if s.pool.QueryRow(s.ctx, `SELECT id::text FROM adrient.groups WHERE org_id=$1::uuid AND name='Studio crew'`, s.org).Scan(&id) == nil {
		return id
	}
	members := "["
	for i, p := range people[:min(len(people), 4)] {
		if i > 0 {
			members += ","
		}
		members += `"` + p + `"`
	}
	members += "]"
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, email, description, member_ids)
		VALUES ($1,$2,'Studio crew','studio-crew@demo.adrient.example','Design and media team',$3::jsonb) RETURNING id::text`, s.org, s.admin, members).Scan(&id); err != nil {
		s.warn("demo group", err)
		return ""
	}
	return id
}

func (s *seeder) driveFolder(parent *string, name string) string {
	var id string
	q := `SELECT id::text FROM adrient.drive_files WHERE org_id=$1::uuid AND name=$2 AND mime_type='application/vnd.adrient.folder' AND trashed_at IS NULL AND `
	if parent == nil {
		if s.pool.QueryRow(s.ctx, q+`parent_id IS NULL`, s.org, name).Scan(&id) == nil {
			return id
		}
	} else if s.pool.QueryRow(s.ctx, q+`parent_id=$3::uuid`, s.org, name, *parent).Scan(&id) == nil {
		return id
	}
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, size_bytes)
		VALUES ($1,$2,$3::uuid,$4,'application/vnd.adrient.folder',0) RETURNING id::text`, s.org, s.admin, parent, name).Scan(&id); err != nil {
		s.warn("drive folder", err)
	}
	return id
}

func (s *seeder) driveFile(parent, name, mime string, data []byte) string {
	key := randKey("drive")
	if err := s.opt.Blobs.Put(s.ctx, key, mime, int64(len(data)), bytes.NewReader(data)); err != nil {
		s.warn("drive blob", err)
		return ""
	}
	var id string
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes)
		VALUES ($1,$2,$3::uuid,$4,$5,$6,$7) RETURNING id::text`, s.org, s.admin, parent, name, mime, key, len(data)).Scan(&id); err != nil {
		s.warn("drive file", err)
	}
	return id
}

func (s *seeder) driveMusic(mp3 []byte, t demoTrack, lrc string) {
	folder := s.driveFolder(nil, "Music")
	if folder == "" {
		return
	}
	s.driveFile(folder, "Drive demo - "+t.title+".mp3", "audio/mpeg", mp3)
	s.driveFile(folder, "Drive demo - "+t.title+".lrc", "text/plain", []byte(lrc))
}

// ---- Video -----------------------------------------------------------------

type demoVideo struct {
	title, desc string
	src         string // lavfi video source
	dur         int
	owner       int // index into people
	vis         string
	tone        float64
	cues        []string
}

const chapterDesc = "\n\nChapters\n0:00 Intro\n0:08 The main idea\n0:16 A closer look\n0:24 Wrapping up"

var demoVideos = []demoVideo{
	{"Welcome to the studio", "A short tour of how the team works, in four parts." + chapterDesc, "testsrc2=s=640x360:r=25", 32, 0, "org", 330,
		[]string{"Welcome, and thanks for joining.", "Here is the main idea in one sentence.", "Let us look at it more closely.", "That is all for this video, see you next time."}},
	{"Quarterly planning walkthrough", "How we plan a quarter: goals, capacity and risks." + chapterDesc, "mandelbrot=s=640x360:r=25", 36, 1, "org", 262,
		[]string{"Planning starts with goals, not tasks.", "Capacity is what the team can really deliver.", "Risks go on the board early.", "Review the plan every two weeks."}},
	{"Onboarding: your first week", "What to expect from day one to day five." + chapterDesc, "smptehdbars=s=640x360:r=25", 30, 2, "org", 392,
		[]string{"Day one is about people and tools.", "Day two is your first small task.", "By day four you ship something.", "Friday is for questions."}},
	{"Product demo: shared calendars", "See how teams share calendars and book rooms." + chapterDesc, "gradients=s=640x360:r=25:speed=0.02:nb_colors=4", 40, 3, "org", 294,
		[]string{"Open the calendar and pick Share.", "Choose a group or a person.", "Rooms show as free or busy.", "Changes sync at once."}},
	{"Design review: navigation", "Notes from the navigation review." + chapterDesc, "rgbtestsrc=s=640x360:r=25", 28, 0, "group", 349,
		[]string{"We reviewed three navigation layouts.", "The left rail tested best.", "Labels beat icons alone.", "Next step is a prototype."}},
	{"Tips for faster mail triage", "Five habits that keep an inbox calm." + chapterDesc, "life=s=640x360:r=25:mold=10:ratio=0.1:death_color=#126C5A:life_color=#F2A21B", 34, 4, "org", 247,
		[]string{"Process mail in batches.", "Use labels, not folders.", "Snooze what can wait.", "Zero is a direction, not a goal."}},
	{"Draft: all-hands recap (private)", "Only visible to me while I edit it." + chapterDesc, "sierpinski=s=640x360:r=25", 26, 0, "private", 311,
		[]string{"Thanks everyone for a strong quarter.", "Three launches landed.", "Two more are coming.", "Questions are welcome."}},
	{"Reading a balance sheet", "A plain-language guide for non-accountants." + chapterDesc, "cellauto=s=640x360:r=25:rule=110:full=1", 38, 5, "org", 370,
		[]string{"Assets are what the business owns.", "Liabilities are what it owes.", "The difference is equity.", "Check the ratios, not just the totals."}},
}

func vtt(cues []string, dur int) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	seg := float64(dur) / float64(len(cues))
	ts := func(t float64) string { return fmt.Sprintf("00:%02d:%06.3f", int(t)/60, math.Mod(t, 60)) }
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, ts(float64(i)*seg+0.5), ts(float64(i+1)*seg-0.5), c)
	}
	return b.String()
}

func (s *seeder) seedVideos(ff, dir string, people []string) {
	gid := s.demoGroup(people)
	var ids []string
	var datas [][]byte
	for i, v := range demoVideos {
		mp4 := filepath.Join(dir, fmt.Sprintf("v%02d.mp4", i))
		err := runFF(s.ctx, ff, "-f", "lavfi", "-i", v.src, "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%.0f:sample_rate=44100", v.tone),
			"-t", fmt.Sprint(v.dur), "-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-pix_fmt", "yuv420p", "-g", "50", "-c:a", "aac", "-b:a", "48k",
			"-af", "volume=0.12", "-movflags", "+faststart", mp4)
		if err != nil {
			s.warn("video clip", err)
			return
		}
		jpg := filepath.Join(dir, fmt.Sprintf("v%02d.jpg", i))
		if err := runFF(s.ctx, ff, "-ss", "2", "-i", mp4, "-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "5", jpg); err != nil {
			s.warn("video poster", err)
			return
		}
		data, _ := os.ReadFile(mp4)
		poster, _ := os.ReadFile(jpg)
		datas = append(datas, data)
		blob := randKey("video")
		if err := s.opt.Blobs.Put(s.ctx, blob, "video/mp4", int64(len(data)), bytes.NewReader(data)); err != nil {
			s.warn("video blob", err)
			return
		}
		owner := people[v.owner%len(people)]
		var g any
		if v.vis == "group" && gid != "" {
			g = gid
		} else if v.vis == "group" {
			v.vis = "org"
		}
		var id string
		if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.videos (org_id, owner_id, title, description, content_type, size, duration_seconds,
			thumbnail_data_url, blob_key, visibility, group_id, created_at)
			VALUES ($1,$2,$3,$4,'video/mp4',$5,$6,$7,$8,$9,$10::uuid, now() - ($11 || ' hours')::interval) RETURNING id::text`,
			s.org, owner, v.title, v.desc, len(data), float64(v.dur), dataURL("image/jpeg", poster), blob, v.vis, g, fmt.Sprint(5+i*26)).Scan(&id); err != nil {
			s.warn("video insert", err)
			return
		}
		ids = append(ids, id)
		s.count("videos", 1)
		capKey := randKey("video") + ".vtt"
		text := vtt(v.cues, v.dur)
		if err := s.opt.Blobs.Put(s.ctx, capKey, "text/vtt", int64(len(text)), strings.NewReader(text)); err == nil {
			s.exec("video caption", `INSERT INTO adrient.video_captions (org_id, video_id, lang, label, blob_key) VALUES ($1,$2,'en','English',$3)`, s.org, id, capKey)
		}
	}
	// Views with watch time, likes, comments, subscriptions, watch later and progress.
	for i, id := range ids {
		for k := 0; k < 3+(i*5)%11; k++ {
			who := people[(i+k)%len(people)]
			s.exec("video view", `INSERT INTO adrient.video_views (video_id, user_id, watched_seconds, created_at)
				VALUES ($1,$2,$3, now() - ($4 || ' hours')::interval)`, id, who, float64(10+(k*7+i*3)%25), fmt.Sprint(k*9+i*4))
		}
		for k := 0; k < 1+(i*3)%4; k++ {
			s.exec("video like", `INSERT INTO adrient.video_likes (video_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, people[(i+k+1)%len(people)])
		}
	}
	bodies := []string{"This was really clear, thank you.", "Could you share the slides afterwards?", "The chapter links make this easy to skim.", "We should try this with the other team.", "Great pace. The last part answered my question."}
	for i, id := range ids[:6] {
		var top string
		if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.video_comments (org_id, video_id, user_id, body, created_at)
			VALUES ($1,$2,$3,$4, now() - interval '3 hours') RETURNING id::text`, s.org, id, people[(i+2)%len(people)], bodies[i%len(bodies)]).Scan(&top); err == nil {
			s.exec("video reply", `INSERT INTO adrient.video_comments (org_id, video_id, user_id, parent_id, body, created_at)
				VALUES ($1,$2,$3,$4,$5, now() - interval '2 hours')`, s.org, id, people[(i+4)%len(people)], top, "Agreed, adding it to our team notes.")
			s.exec("comment like", `INSERT INTO adrient.video_comment_likes (comment_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, top, s.admin)
			s.count("video comments", 2)
		}
	}
	for _, c := range people[1:min(len(people), 5)] {
		s.exec("subscription", `INSERT INTO adrient.video_subscriptions (user_id, channel_user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, s.admin, c)
		s.exec("subscription", `INSERT INTO adrient.video_subscriptions (user_id, channel_user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, people[(len(c))%len(people)], s.admin)
	}
	for _, i := range []int{1, 3} {
		s.exec("watch later", `INSERT INTO adrient.video_watch_later (user_id, video_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, s.admin, ids[i])
	}
	for i, pos := range []float64{14, 20, 9} {
		s.exec("video progress", `INSERT INTO adrient.video_progress (user_id, video_id, position_seconds, percent, watched, updated_at)
			VALUES ($1,$2,$3,$4,false, now() - ($5 || ' hours')::interval) ON CONFLICT DO NOTHING`, s.admin, ids[i+1], pos, pos/float64(demoVideos[i+1].dur), fmt.Sprint(i+1))
	}
	// One Meet recording, saved to Drive, for the Meetings shelf.
	var room string
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO adrient.meet_rooms (org_id, owner_id, name) VALUES ($1,$2,'Weekly design sync') RETURNING id::text`, s.org, s.admin).Scan(&room); err == nil {
		folder := s.driveFolder(nil, "Meet recordings")
		if f := s.driveFile(folder, "Weekly design sync - recording.mp4", "video/mp4", datas[2]); f != "" {
			s.exec("meet recording", `INSERT INTO adrient.meet_recordings (room_id, org_id, started_by, egress_id, object_key, status, drive_file_id, started_at, finished_at)
				VALUES ($1,$2,$3,$4,'demo/recording.mp4','saved',$5, now() - interval '30 hours', now() - interval '29 hours')`, room, s.org, s.admin, "demo-egress-"+room, f)
			s.count("meet recordings", 1)
		}
	}
}
