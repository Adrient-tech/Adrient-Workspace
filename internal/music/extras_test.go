package music

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// id3Frame builds one ID3v2.3 frame.
func id3Frame(id string, payload []byte) []byte {
	n := len(payload)
	h := []byte(id)
	h = append(h, byte(n>>24), byte(n>>16), byte(n>>8), byte(n), 0, 0)
	return append(h, payload...)
}

func syncsafeBytes(n int) []byte {
	return []byte{byte(n >> 21 & 0x7f), byte(n >> 14 & 0x7f), byte(n >> 7 & 0x7f), byte(n & 0x7f)}
}

func textPayload(s string) []byte { return append([]byte{3}, []byte(s)...) } // UTF-8

func utf16Payload(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := []byte{1, 0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

func buildTag(frames ...[]byte) []byte {
	body := bytes.Join(frames, nil)
	tag := append([]byte("ID3"), 3, 0, 0)
	tag = append(tag, syncsafeBytes(len(body))...)
	return append(tag, body...)
}

func TestParseID3v2ReadsTextFramesAndCover(t *testing.T) {
	pic := append([]byte{0}, []byte("image/jpeg")...)
	pic = append(pic, 0, 3, 0) // terminator, picture type, empty description
	pic = append(pic, []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}...)
	lyr := append([]byte{3, 'e', 'n', 'g'}, 0) // empty descriptor
	lyr = append(lyr, []byte("[00:01.00]hello")...)
	tag := buildTag(
		id3Frame("TIT2", textPayload("Morning Ferry")),
		id3Frame("TPE1", utf16Payload("Lumen Harbor")),
		id3Frame("TALB", textPayload("Slow Tide")),
		id3Frame("TCON", textPayload("(13)Ambient")),
		id3Frame("TRCK", textPayload("2/9")),
		id3Frame("TYER", textPayload("2024")),
		id3Frame("USLT", lyr),
		id3Frame("APIC", pic),
	)
	got := ReadTags(bytes.NewReader(append(tag, []byte("audio bytes follow")...)))
	if got.Title != "Morning Ferry" || got.Artist != "Lumen Harbor" || got.Album != "Slow Tide" {
		t.Fatalf("text frames: %+v", got)
	}
	if got.Genre != "Ambient" || got.TrackNo != 2 || got.Year != 2024 {
		t.Fatalf("numbers: genre=%q track=%d year=%d", got.Genre, got.TrackNo, got.Year)
	}
	if !IsLRC(got.Lyrics) || !strings.Contains(got.Lyrics, "hello") {
		t.Fatalf("lyrics = %q", got.Lyrics)
	}
	if got.ArtMIME != "image/jpeg" || len(got.ArtBytes) != 7 || !strings.HasPrefix(got.ArtDataURL(), "data:image/jpeg;base64,") {
		t.Fatalf("cover: %q %d", got.ArtMIME, len(got.ArtBytes))
	}
}

func TestParseID3v2IgnoresNonTagsAndTruncation(t *testing.T) {
	if _, ok := ParseID3v2([]byte("not a tag at all")); ok {
		t.Fatal("plain bytes must not parse")
	}
	if tg := ReadTags(strings.NewReader("")); tg.Title != "" {
		t.Fatal("empty input must give empty tags")
	}
	tag := buildTag(id3Frame("TIT2", textPayload("Kept")), id3Frame("TPE1", textPayload("Cut off here")))
	got := ReadTags(bytes.NewReader(tag[:len(tag)-8])) // truncated in the middle of the second frame
	if got.Title != "Kept" {
		t.Fatalf("title before the damage must survive, got %q", got.Title)
	}
}

func TestIsLRC(t *testing.T) {
	if !IsLRC("[ti:x]\n[00:12.30]line") || IsLRC("just words\nno stamps") {
		t.Fatal("IsLRC wrong")
	}
}

func TestDecodeDataURL(t *testing.T) {
	b, mime, _, err := DecodeDataURL("data:image/png;base64,aGVsbG8=", time.Time{})
	if err != nil || mime != "image/png" || string(b) != "hello" {
		t.Fatalf("got %q %q %v", b, mime, err)
	}
	for _, bad := range []string{"", "http://x", "data:text/html;base64,aGk=", "data:image/svg+xml;base64,aGk=", "data:image/png;base64,!!"} {
		if _, _, _, err := DecodeDataURL(bad, time.Time{}); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

// ---- database tests (skipped without ADRIENT_TEST_DSN) ---------------------

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
	return io.NopCloser(bytes.NewReader(b)), "audio/mpeg", int64(len(b)), nil
}
func (m memBlobs) Delete(_ context.Context, key string) error { delete(m, key); return nil }

func TestExtrasPlaysSmartListsAndLikes(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	ex := NewExtras(pool, memBlobs{})
	a, _ := repo.CreateTrack(ctx, org, user, CreateTrackParams{Title: "A", BlobKey: "k1", ContentType: "audio/mpeg"})
	b, _ := repo.CreateTrack(ctx, org, user, CreateTrackParams{Title: "B", BlobKey: "k2", ContentType: "audio/mpeg"})
	for i := 0; i < 3; i++ {
		if err := ex.RecordPlay(ctx, org, user, b.ID, 31); err != nil {
			t.Fatal(err)
		}
	}
	if err := ex.RecordPlay(ctx, org, user, a.ID, 31); err != nil {
		t.Fatal(err)
	}
	if err := ex.RecordPlay(ctx, org, user, "00000000-0000-0000-0000-000000000000", 1); err != ErrNotFound {
		t.Fatalf("unknown track: %v", err)
	}
	most, err := ex.Smart(ctx, org, user, "most_played", 10)
	if err != nil || len(most) != 2 || most[0].Title != "B" || most[0].PlayCount != 3 {
		t.Fatalf("most played: %+v %v", most, err)
	}
	recent, _ := ex.Smart(ctx, org, user, "recently_played", 10)
	if len(recent) != 2 || recent[0].Title != "A" {
		t.Fatalf("recently played should start with the last played: %+v", recent)
	}
	added, _ := ex.Smart(ctx, org, user, "recently_added", 10)
	if len(added) != 2 || added[0].Title != "B" {
		t.Fatalf("recently added newest first: %+v", added)
	}
	if err := repo.LikeTrack(ctx, org, user, a.ID); err != nil {
		t.Fatal(err)
	}
	liked, _ := ex.Smart(ctx, org, user, "liked", 10)
	if len(liked) != 1 || liked[0].Title != "A" || !liked[0].Liked {
		t.Fatalf("liked: %+v", liked)
	}
	if _, err := ex.Smart(ctx, org, user, "bogus", 10); err == nil {
		t.Fatal("unknown smart list must fail")
	}
	hist, _ := ex.History(ctx, org, user, 10)
	if len(hist) != 4 {
		t.Fatalf("history has one row per play, got %d", len(hist))
	}
}

func TestExtrasLyricsAreOwnerOnly(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	tr, _ := NewRepository(pool).CreateTrack(ctx, org, user, CreateTrackParams{Title: "A", BlobKey: "k", ContentType: "audio/mpeg"})
	ex := NewExtras(pool, memBlobs{})
	if err := ex.SetLyrics(ctx, org, user, tr.ID, "[00:01.00]hi"); err != nil {
		t.Fatal(err)
	}
	l, err := ex.Lyrics(ctx, org, tr.ID)
	if err != nil || !IsLRC(l) {
		t.Fatalf("lyrics %q %v", l, err)
	}
	if err := ex.SetLyrics(ctx, org, "11111111-1111-1111-1111-111111111111", tr.ID, "x"); err != ErrNotFound {
		t.Fatalf("a non-owner must not edit lyrics: %v", err)
	}
}

func TestExtrasPlaylistVisibilityAndOrder(t *testing.T) {
	pool, org, owner := setupDB(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	var other string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name)
		VALUES ($1,'test','subject-2','other@adrient.localtest.me','Other') RETURNING id::text`, org).Scan(&other); err != nil {
		t.Fatal(err)
	}
	ex := NewExtras(pool, memBlobs{})
	var ids []string
	for _, n := range []string{"one", "two", "three"} {
		tr, _ := repo.CreateTrack(ctx, org, owner, CreateTrackParams{Title: n, BlobKey: "k" + n, ContentType: "audio/mpeg"})
		ids = append(ids, tr.ID)
	}
	pl, _ := repo.CreatePlaylist(ctx, org, owner, PlaylistFields{Name: "Mine"})
	for _, id := range ids {
		if _, err := repo.AddTrackToPlaylist(ctx, org, pl.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	// Existing playlists stay visible to the organisation until their owner changes that.
	if l, _ := ex.Playlists(ctx, org, other); len(l) != 1 || l[0].Visibility != "org" || l[0].Mine {
		t.Fatalf("legacy playlist should be org-visible: %+v", l)
	}
	if err := ex.SetPlaylistSharing(ctx, org, other, pl.ID, "private", ""); err != ErrNotFound {
		t.Fatalf("only the owner can change sharing: %v", err)
	}
	if err := ex.SetPlaylistSharing(ctx, org, owner, pl.ID, "private", ""); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.Playlists(ctx, org, other); len(l) != 0 {
		t.Fatalf("private playlist leaked: %+v", l)
	}
	if _, _, err := ex.Playlist(ctx, org, other, pl.ID); err != ErrNotFound {
		t.Fatalf("private playlist detail leaked: %v", err)
	}
	// Group sharing needs membership.
	var gid string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids) VALUES ($1,$2,'Crew',$3::jsonb) RETURNING id::text`,
		org, owner, `["`+other+`"]`).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	if err := ex.SetPlaylistSharing(ctx, org, owner, pl.ID, "group", ""); err == nil {
		t.Fatal("group sharing without a group must fail")
	}
	if err := ex.SetPlaylistSharing(ctx, org, owner, pl.ID, "group", gid); err != nil {
		t.Fatal(err)
	}
	if l, _ := ex.Playlists(ctx, org, other); len(l) != 1 || l[0].GroupName != "Crew" {
		t.Fatalf("group member should see the playlist: %+v", l)
	}
	// Reorder.
	if err := ex.ReorderPlaylist(ctx, org, owner, pl.ID, []string{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}
	_, tr, _ := ex.Playlist(ctx, org, owner, pl.ID)
	if len(tr) != 3 || tr[0].Title != "three" || tr[2].Title != "two" {
		t.Fatalf("order: %+v", tr)
	}
	if err := ex.ReorderPlaylist(ctx, org, owner, pl.ID, ids[:2]); err == nil {
		t.Fatal("a partial list must be refused")
	}
	if err := ex.ReorderPlaylist(ctx, org, other, pl.ID, ids); err != ErrNotFound {
		t.Fatalf("non-owner reorder: %v", err)
	}
	// The gRPC listing hides it from non-members too.
	if err := ex.SetPlaylistSharing(ctx, org, owner, pl.ID, "private", ""); err != nil {
		t.Fatal(err)
	}
	all, _ := repo.ListPlaylists(ctx, org)
	if got := repo.filterVisible(ctx, org, other, all); len(got) != 0 {
		t.Fatalf("filterVisible leaked %d", len(got))
	}
	if got := repo.filterVisible(ctx, org, owner, all); len(got) != 1 {
		t.Fatalf("owner must still see it, got %d", len(got))
	}
}

func TestExtrasImportFromDriveReadsTagsAndLyricsWithoutCopying(t *testing.T) {
	pool, org, user := setupDB(t)
	ctx := context.Background()
	blobs := memBlobs{}
	mp3 := append(buildTag(id3Frame("TIT2", textPayload("Drive Song")), id3Frame("TPE1", textPayload("Drive Artist"))), []byte("audio")...)
	blobs["drive/song"] = mp3
	blobs["drive/lrc"] = []byte("[00:02.00]from drive")
	var folder, song string
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type) VALUES ($1,$2,'Music','application/vnd.adrient.folder') RETURNING id::text`, org, user).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes)
		VALUES ($1,$2,$3,'song.mp3','audio/mpeg','drive/song',$4) RETURNING id::text`, org, user, folder, len(mp3)).Scan(&song); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes)
		VALUES ($1,$2,$3,'song.lrc','text/plain','drive/lrc',21)`, org, user, folder); err != nil {
		t.Fatal(err)
	}
	ex := NewExtras(pool, blobs)
	files, _ := ex.DriveAudioFiles(ctx, org, user, "")
	if len(files) != 1 || files[0].Name != "song.mp3" || files[0].Imported {
		t.Fatalf("drive list: %+v", files)
	}
	tr, err := ex.ImportFromDrive(ctx, org, user, song)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Title != "Drive Song" || tr.Artist != "Drive Artist" || !tr.HasLyrics {
		t.Fatalf("imported: %+v", tr)
	}
	again, _ := ex.ImportFromDrive(ctx, org, user, song)
	if again.ID != tr.ID {
		t.Fatal("import must be idempotent")
	}
	// Deleting the imported track must leave the Drive file's bytes alone.
	key, err := NewRepository(pool).TrashTrack(ctx, org, tr.ID)
	if err != nil || key != "" {
		t.Fatalf("trash of a Drive-backed track must not return a blob to delete: %q %v", key, err)
	}
	if _, ok := blobs["drive/song"]; !ok {
		t.Fatal("drive blob vanished")
	}
	if _, err := ex.ImportFromDrive(ctx, org, user, folder); err == nil {
		t.Fatal("a folder is not importable")
	}
}
