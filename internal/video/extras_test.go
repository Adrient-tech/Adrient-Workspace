package video

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

type memBlobs map[string][]byte

func (m memBlobs) Put(_ context.Context, key, _ string, _ int64, r io.Reader) error {
	b, _ := io.ReadAll(r)
	m[key] = b
	return nil
}
func (m memBlobs) Get(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	b, ok := m[key]
	if !ok {
		return nil, "", 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), "video/mp4", int64(len(b)), nil
}
func (m memBlobs) Delete(_ context.Context, key string) error { delete(m, key); return nil }

func secondUser(t *testing.T, ex *Extras, org string) string {
	t.Helper()
	var id string
	if err := ex.pool.QueryRow(context.Background(), `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-2','viewer@adrient.localtest.me','Viewer') RETURNING id::text`, org).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestExtrasVisibility(t *testing.T) {
	pool, org, owner := setupDB(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	ex := NewExtras(pool, memBlobs{})
	other := secondUser(t, ex, org)
	v, _ := repo.Create(ctx, org, owner, sampleParams())

	// Existing rows keep today's behaviour: visible to the whole organisation.
	if l, _ := ex.Feed(ctx, org, other, FeedFilter{}); len(l) != 1 {
		t.Fatalf("legacy video should be org-visible, got %d", len(l))
	}
	priv := "private"
	if _, err := ex.Update(ctx, org, owner, v.ID, UpdateInput{Visibility: priv}); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.Feed(ctx, org, other, FeedFilter{}); len(l) != 0 {
		t.Fatal("private video leaked into another user's feed")
	}
	if repo.CanView(ctx, org, other, v.ID) || !repo.CanView(ctx, org, owner, v.ID) {
		t.Fatal("CanView wrong for private video")
	}
	if _, err := ex.One(ctx, org, other, v.ID); err != ErrNotFound {
		t.Fatalf("detail of a private video: %v", err)
	}
	if got := repo.visibleOnly(ctx, org, other, []Video{v}); len(got) != 0 {
		t.Fatal("gRPC list filter leaked a private video")
	}
	if _, err := ex.Update(ctx, org, other, v.ID, UpdateInput{Visibility: "org"}); err != ErrNotFound {
		t.Fatalf("only the owner may change visibility: %v", err)
	}
	// Group visibility needs a group the owner belongs to; members then see it.
	if _, err := ex.Update(ctx, org, owner, v.ID, UpdateInput{Visibility: "group"}); err == nil {
		t.Fatal("group without group_id must fail")
	}
	var gid string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids) VALUES ($1,$2,'Crew',$3::jsonb) RETURNING id::text`, org, owner, `["`+other+`"]`).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Update(ctx, org, owner, v.ID, UpdateInput{Visibility: "group", GroupID: gid}); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.Feed(ctx, org, other, FeedFilter{}); len(l) != 1 {
		t.Fatal("group member should see the video")
	}
	if _, err := ex.Update(ctx, org, owner, v.ID, UpdateInput{Visibility: "secret"}); err == nil {
		t.Fatal("unknown visibility must fail")
	}
}

func TestExtrasFeedFilters(t *testing.T) {
	pool, org, owner := setupDB(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	ex := NewExtras(pool, memBlobs{})
	mk := func(title string, dur float64) Video {
		p := sampleParams()
		p.Title, p.DurationSeconds = title, dur
		v, err := repo.Create(ctx, org, owner, p)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	short, long := mk("Quick tip", 90), mk("Deep dive planning", 1800)
	mk("Medium talk", 600)
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Duration: "short"}); len(l) != 1 || l[0].ID != short.ID {
		t.Fatalf("short filter: %+v", l)
	}
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Duration: "long"}); len(l) != 1 || l[0].ID != long.ID {
		t.Fatalf("long filter: %+v", l)
	}
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Q: "planning"}); len(l) != 1 || l[0].ID != long.ID {
		t.Fatalf("search: %+v", l)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.videos SET created_at = now() - interval '40 days' WHERE id=$1`, long.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Date: "week"}); len(l) != 2 {
		t.Fatalf("this week should skip the old video, got %d", len(l))
	}
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Sort: "title"}); l[0].Title != "Deep dive planning" {
		t.Fatalf("title sort: %s", l[0].Title)
	}
}

func TestExtrasViewsLikesCommentsSubscriptions(t *testing.T) {
	pool, org, owner := setupDB(t)
	ctx := context.Background()
	ex := NewExtras(pool, memBlobs{})
	other := secondUser(t, ex, org)
	v, _ := NewRepository(pool).Create(ctx, org, owner, sampleParams())

	if err := ex.RecordView(ctx, org, other, v.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := ex.RecordView(ctx, org, other, v.ID, 25); err != nil { // within 30 minutes: adds time, not a new view
		t.Fatal(err)
	}
	st, err := ex.Stats(ctx, org, owner, v.ID)
	if err != nil || st.Views != 1 || st.UniqueViewers != 1 || st.WatchSeconds != 25 || len(st.Daily) != 14 {
		t.Fatalf("stats: %+v %v", st, err)
	}
	if _, err := ex.Stats(ctx, org, other, v.ID); err != ErrNotFound {
		t.Fatalf("stats are owner-only: %v", err)
	}

	if err := ex.SetLike(ctx, org, other, v.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := ex.SetLike(ctx, org, other, v.ID, true); err != nil { // idempotent
		t.Fatal(err)
	}
	info, _ := ex.One(ctx, org, other, v.ID)
	if info.Likes != 1 || !info.Liked || info.Views != 1 {
		t.Fatalf("info: %+v", info)
	}

	top, err := ex.AddComment(ctx, org, other, v.ID, "", "  Great video 1:30  ")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := ex.AddComment(ctx, org, owner, v.ID, top, "Thanks!")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := ex.AddComment(ctx, org, other, v.ID, reply, "Reply to a reply")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ex.AddComment(ctx, org, other, v.ID, "", "   "); err == nil {
		t.Fatal("empty comment must be refused")
	}
	cs, _ := ex.Comments(ctx, org, owner, v.ID)
	if len(cs) != 3 {
		t.Fatalf("comments: %d", len(cs))
	}
	for _, c := range cs {
		if c.ID == nested && c.ParentID != top {
			t.Fatal("replies must attach to the top-level comment")
		}
		if c.ID == top && c.Body != "Great video 1:30" {
			t.Fatalf("body not trimmed: %q", c.Body)
		}
		if c.ID == top && !c.CanDelete { // the video owner can moderate
			t.Fatal("video owner should be able to delete comments")
		}
	}
	on, n, err := ex.ToggleCommentLike(ctx, org, owner, top)
	if err != nil || !on || n != 1 {
		t.Fatalf("like: %v %d %v", on, n, err)
	}
	on, n, _ = ex.ToggleCommentLike(ctx, org, owner, top)
	if on || n != 0 {
		t.Fatal("second toggle should remove the like")
	}
	if err := ex.DeleteComment(ctx, org, "22222222-2222-2222-2222-222222222222", top); err != ErrNotFound {
		t.Fatalf("a stranger deleted a comment: %v", err)
	}
	if err := ex.DeleteComment(ctx, org, owner, top); err != nil {
		t.Fatal(err)
	}
	if cs, _ := ex.Comments(ctx, org, owner, v.ID); len(cs) != 0 {
		t.Fatalf("deleting a comment removes its replies, %d left", len(cs))
	}

	// Subscriptions.
	if err := ex.Subscribe(ctx, org, owner, owner, true); err == nil {
		t.Fatal("cannot subscribe to yourself")
	}
	if err := ex.Subscribe(ctx, org, other, owner, true); err != nil {
		t.Fatal(err)
	}
	ch, _ := ex.Channel(ctx, org, other, owner)
	if ch.Subscribers != 1 || !ch.Subscribed || ch.Videos != 1 {
		t.Fatalf("channel: %+v", ch)
	}
	if l, _ := ex.Feed(ctx, org, other, FeedFilter{Subs: true}); len(l) != 1 {
		t.Fatal("subscription feed")
	}
	if l, _ := ex.Feed(ctx, org, owner, FeedFilter{Subs: true}); len(l) != 0 {
		t.Fatal("owner subscribes to nobody")
	}

	// Watch later and history.
	if err := ex.SetWatchLater(ctx, org, other, v.ID, true); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.WatchLater(ctx, org, other); len(l) != 1 || !l[0].WatchLater {
		t.Fatal("watch later list")
	}
	if _, err := NewProgressRepository(pool).Upsert(ctx, other, v.ID, 12, 0.3); err != nil {
		t.Fatal(err)
	}
	h, _ := ex.History(ctx, org, other)
	if len(h) != 1 || h[0].ProgressSecs != 12 {
		t.Fatalf("history keeps the position: %+v", h)
	}
	if err := ex.ClearHistory(ctx, other); err != nil {
		t.Fatal(err)
	}
	if h, _ := ex.History(ctx, org, other); len(h) != 0 {
		t.Fatal("history not cleared")
	}
}

func TestExtrasDriveImportMeetingAndCaptions(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	blobs := memBlobs{"drive/talk": []byte("video bytes"), "drive/talk-vtt": []byte("WEBVTT\n\n1\n00:00:00.000 --> 00:00:02.000\nHi\n")}
	ex := NewExtras(pool, blobs)
	var folder, file string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type) VALUES ($1,$2,'Meet recordings','application/vnd.adrient.folder') RETURNING id::text`, org, user).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes)
		VALUES ($1,$2,$3,'talk.mp4','video/mp4','drive/talk',11) RETURNING id::text`, org, user, folder).Scan(&file); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes)
		VALUES ($1,$2,$3,'talk.vtt','text/vtt','drive/talk-vtt',40)`, org, user, folder); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.DriveVideoFiles(ctx, org, user, ""); len(l) != 1 || l[0].Name != "talk.mp4" {
		t.Fatalf("drive list: %+v", l)
	}
	v, err := ex.ImportFromDrive(ctx, org, user, file, ImportOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if v.Visibility != "private" || v.Title != "talk" || !v.HasCaptions {
		t.Fatalf("imported video: %+v", v)
	}
	if again, _ := ex.ImportFromDrive(ctx, org, user, file, ImportOpts{}); again.ID != v.ID {
		t.Fatal("import must be idempotent")
	}
	if key, err := NewRepository(pool).Trash(ctx, org, v.ID); err != nil || key != "" {
		t.Fatalf("Drive-backed video must not delete the Drive blob: %q %v", key, err)
	}

	// A Meet recording becomes a private video named after the room.
	var room string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.meet_rooms (org_id, owner_id, name) VALUES ($1,$2,'Design sync') RETURNING id::text`, org, user).Scan(&room); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.meet_recordings (room_id, org_id, started_by, egress_id, object_key, status, drive_file_id, started_at)
		VALUES ($1,$2,$3,'e1','k','saved',$4,$5)`, room, org, user, file, time.Now()); err != nil {
		t.Fatal(err)
	}
	recs, _ := ex.Meetings(ctx, org, user)
	if len(recs) != 1 || recs[0].Room != "Design sync" {
		t.Fatalf("meetings: %+v", recs)
	}
	mv, err := ex.OpenMeeting(ctx, org, user, recs[0].ID)
	if err != nil || mv.SourceKind != "meet" || mv.Visibility != "private" {
		t.Fatalf("open meeting: %+v %v", mv, err)
	}
	if again, _ := ex.OpenMeeting(ctx, org, user, recs[0].ID); again.ID != mv.ID {
		t.Fatal("opening twice must reuse the video")
	}
	if _, err := ex.OpenMeeting(ctx, org, "33333333-3333-3333-3333-333333333333", recs[0].ID); err != ErrNotFound {
		t.Fatalf("a stranger opened someone's recording: %v", err)
	}
}
