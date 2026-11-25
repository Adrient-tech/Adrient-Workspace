package chat

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExtractMentions(t *testing.T) {
	members := []orgUser{
		{ID: "a", Name: "Asha Rao", Email: "asha@x.io"},
		{ID: "b", Name: "Bob", Email: "bob@x.io"},
		{ID: "c", Name: "Bobby Tables", Email: "bobby@x.io"},
	}
	got := ExtractMentions("hi @Asha Rao and @bob, ok?", members, "z")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
	// "@bobby" must not also match "@bob" (token boundary), sender excluded.
	got = ExtractMentions("ping @bobby@x.io", members, "z")
	if len(got) != 1 || got[0] != "c" {
		t.Fatalf("got %v", got)
	}
	got = ExtractMentions("@channel heads up", members, "a")
	if len(got) != 2 {
		t.Fatalf("channel mention: %v", got)
	}
	if got = ExtractMentions("no mentions", members, "z"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestNormalizeChannelName(t *testing.T) {
	if got := NormalizeChannelName("  Team Launch!! "); got != "team-launch" {
		t.Fatalf("got %q", got)
	}
}

func TestTeamsStoreFlow(t *testing.T) {
	pool, orgID, a := setupDB(t)
	b := seedSecondUser(t, pool, orgID)
	ctx := context.Background()
	st := NewStore(NewRepository(pool))

	// Public channel: B is not a member until join; duplicate names conflict.
	gen, err := st.CreateTChannel(ctx, orgID, a, "public", "General", "t", "d", nil)
	if err != nil || gen.Name != "general" || gen.Kind != "public" {
		t.Fatalf("create public: %+v %v", gen, err)
	}
	if _, err := st.CreateTChannel(ctx, orgID, a, "public", "general", "", "", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	if _, err := st.Post(ctx, orgID, b, "Other", gen.ID, "", "hi", nil, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-member post: %v", err)
	}
	br, _ := st.BrowseChannels(ctx, orgID, b)
	if len(br) != 1 || br[0].IsMember {
		t.Fatalf("browse: %+v", br)
	}
	if err := st.AddMembers(ctx, orgID, gen.ID, []string{b}); err != nil {
		t.Fatal(err)
	}

	// Mention + unread/mention counts for B, none for the sender.
	res, err := st.Post(ctx, orgID, a, "Tester", gen.ID, "", "hello @Other welcome", nil, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Message.MentionIDs) != 1 || res.Message.MentionIDs[0] != b || res.Message.ClientNonce != "n1" {
		t.Fatalf("mentions: %+v", res.Message)
	}
	cb, _ := st.GetChannel(ctx, orgID, b, gen.ID)
	if cb.UnreadCount != 1 || cb.MentionCount != 1 {
		t.Fatalf("B counts: %+v", cb)
	}
	ca, _ := st.GetChannel(ctx, orgID, a, gen.ID)
	if ca.UnreadCount != 0 {
		t.Fatalf("sender unread: %d", ca.UnreadCount)
	}
	act, _ := st.Activity(ctx, orgID, b)
	if len(act) != 1 || act[0].Kind != "mention" || !act[0].Unread {
		t.Fatalf("activity: %+v", act)
	}
	if n := st.ActivityUnread(ctx, orgID, b); n != 1 {
		t.Fatalf("activity unread %d", n)
	}
	_ = st.MarkActivitySeen(ctx, b)
	if n := st.ActivityUnread(ctx, orgID, b); n != 0 {
		t.Fatalf("after seen %d", n)
	}
	_ = st.repo.UpdateReadCursor(ctx, gen.ID, b)

	// Thread reply by B shows up in A's activity (A started the thread).
	rep, err := st.Post(ctx, orgID, b, "Other", gen.ID, res.Message.ID, "thanks", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Message.ParentID != res.Message.ID {
		t.Fatalf("parent: %+v", rep.Message)
	}
	found := false
	for _, id := range rep.Notify {
		if id == a {
			found = true
		}
	}
	if !found {
		t.Fatalf("thread parent author not notified: %v", rep.Notify)
	}
	act, _ = st.Activity(ctx, orgID, a)
	if len(act) != 1 || act[0].Kind != "thread_reply" {
		t.Fatalf("A activity: %+v", act)
	}
	th, _ := st.ListThread(ctx, orgID, a, res.Message.ID)
	if len(th) != 1 {
		t.Fatalf("thread len %d", len(th))
	}
	top, _, _ := st.ListChannelMessages(ctx, orgID, a, gen.ID, "", "", nil, 50)
	if len(top) != 1 || top[0].ReplyCount != 1 {
		t.Fatalf("top-level: %+v", top)
	}

	// Edit (sender only), reactions, pin, save, search.
	if _, err := st.EditMessage(ctx, orgID, b, res.Message.ID, "x"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("edit by other: %v", err)
	}
	ed, err := st.EditMessage(ctx, orgID, a, res.Message.ID, "hello again")
	if err != nil || ed.EditedAt == "" || len(ed.MentionIDs) != 0 {
		t.Fatalf("edit: %+v %v", ed, err)
	}
	if _, err := st.repo.ToggleReaction(ctx, orgID, res.Message.ID, b, "👍"); err != nil {
		t.Fatal(err)
	}
	pinned, _ := st.TogglePin(ctx, a, gen.ID, res.Message.ID)
	saved, _ := st.ToggleSaved(ctx, b, res.Message.ID)
	if !pinned || !saved {
		t.Fatal("pin/save should be on")
	}
	pins, _ := st.ListPins(ctx, orgID, b, gen.ID)
	sv, _ := st.ListSaved(ctx, orgID, b)
	if len(pins) != 1 || len(pins[0].Reactions) != 1 || !pins[0].Reactions[0].Me || !pins[0].Saved || len(sv) != 1 {
		t.Fatalf("pins/saved: %+v %+v", pins, sv)
	}
	hits, _ := st.Search(ctx, orgID, b, "AGAIN", "", "", nil, nil)
	if len(hits) != 1 {
		t.Fatalf("search: %d", len(hits))
	}
	// Search never leaks channels the user is not in.
	priv, _ := st.CreateTChannel(ctx, orgID, a, "private", "secret", "", "", nil)
	_, _ = st.Post(ctx, orgID, a, "Tester", priv.ID, "", "needle", nil, "")
	if hits, _ = st.Search(ctx, orgID, b, "needle", "", "", nil, nil); len(hits) != 0 {
		t.Fatalf("leak: %+v", hits)
	}
	if hits, _ = st.Search(ctx, orgID, a, "needle", "", "", nil, nil); len(hits) != 1 {
		t.Fatalf("own search: %d", len(hits))
	}

	// Around / from-date paging.
	around, _, err := st.ListChannelMessages(ctx, orgID, a, gen.ID, "", res.Message.ID, nil, 50)
	if err != nil || len(around) != 1 {
		t.Fatalf("around: %v %d", err, len(around))
	}
	tomorrow := time.Now().Add(24 * time.Hour)
	if from, _, _ := st.ListChannelMessages(ctx, orgID, a, gen.ID, "", "", &tomorrow, 50); len(from) != 0 {
		t.Fatalf("from tomorrow: %d", len(from))
	}

	// Sidebar prefs and sections.
	sec, err := st.CreateSection(ctx, orgID, a, "Projects")
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := st.SetPrefs(ctx, a, gen.ID, &yes, nil, &sec.ID); err != nil {
		t.Fatal(err)
	}
	ca, _ = st.GetChannel(ctx, orgID, a, gen.ID)
	if !ca.Starred || ca.SectionID != sec.ID {
		t.Fatalf("prefs: %+v", ca)
	}
	empty := ""
	_ = st.SetPrefs(ctx, a, gen.ID, nil, &yes, &empty)
	ca, _ = st.GetChannel(ctx, orgID, a, gen.ID)
	if !ca.Starred || !ca.Muted || ca.SectionID != "" {
		t.Fatalf("prefs 2: %+v", ca)
	}

	// DM and group DM dedupe.
	dm1, _ := st.CreateTChannel(ctx, orgID, a, "dm", "", "", "", []string{b})
	dm2, _ := st.CreateTChannel(ctx, orgID, b, "dm", "", "", "", []string{a})
	if dm1.ID != dm2.ID || dm1.Kind != "dm" {
		t.Fatalf("dm dedupe: %s %s", dm1.ID, dm2.ID)
	}

	// Drafts, scheduled sends and reminders.
	if err := st.SaveDraft(ctx, a, gen.ID, "", "half-typed"); err != nil {
		t.Fatal(err)
	}
	if ds, _ := st.ListDrafts(ctx, a); len(ds) != 1 {
		t.Fatalf("drafts %d", len(ds))
	}
	if _, err := st.Schedule(ctx, orgID, a, gen.ID, "", "later", time.Now().Add(-time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past schedule: %v", err)
	}
	sc, err := st.Schedule(ctx, orgID, a, gen.ID, "", "scheduled hello", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.chat_scheduled SET send_at = now() - interval '1 second' WHERE id=$1::uuid`, sc.ID); err != nil {
		t.Fatal(err)
	}
	rm, err := st.AddReminder(ctx, orgID, b, res.Message.ID, "follow up", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE adrient.chat_reminders SET remind_at = now() - interval '1 second' WHERE id=$1::uuid`, rm.ID); err != nil {
		t.Fatal(err)
	}
	NewTeamsHandler(st, NewHub()).Tick(ctx)
	top, _, _ = st.ListChannelMessages(ctx, orgID, a, gen.ID, "", "", nil, 50)
	if len(top) != 2 || top[1].Body != "scheduled hello" {
		t.Fatalf("scheduled not delivered: %+v", top)
	}
	if xs, _ := st.ListScheduled(ctx, a); len(xs) != 0 {
		t.Fatalf("still pending: %d", len(xs))
	}
	act, _ = st.Activity(ctx, orgID, b)
	hasRem := false
	for _, it := range act {
		if it.Kind == "reminder" && it.Note == "follow up" {
			hasRem = true
		}
	}
	if !hasRem {
		t.Fatalf("reminder missing from activity: %+v", act)
	}

	// Leave, then the channel disappears from B's list; org isolation of people.
	if err := st.RemoveMember(ctx, orgID, gen.ID, b); err != nil {
		t.Fatal(err)
	}
	if chs, _ := st.ListChannels(ctx, orgID, b); len(chs) != 1 /* only the DM */ {
		t.Fatalf("B channels after leave: %d", len(chs))
	}
	ppl, _ := st.People(ctx, orgID)
	if len(ppl) != 2 {
		t.Fatalf("people %d", len(ppl))
	}
}

func TestHuddleCodeFirstWins(t *testing.T) {
	pool, orgID, a := setupDB(t)
	ctx := context.Background()
	st := NewStore(NewRepository(pool))
	ch, _ := st.CreateTChannel(ctx, orgID, a, "public", "huddles", "", "", nil)
	c1, _ := st.SetHuddle(ctx, orgID, ch.ID, "abc-defg-hij")
	c2, _ := st.SetHuddle(ctx, orgID, ch.ID, "zzz-zzzz-zzz")
	if c1 != "abc-defg-hij" || c2 != "abc-defg-hij" {
		t.Fatalf("huddle codes: %s %s", c1, c2)
	}
}
