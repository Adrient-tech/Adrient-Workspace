import { useCallback, useEffect, useState } from "react";
import { Box } from "@mui/joy";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import AutoStoriesOutlined from "@mui/icons-material/AutoStoriesOutlined";
import HeadphonesOutlined from "@mui/icons-material/HeadphonesOutlined";
import BookmarkBorderRounded from "@mui/icons-material/BookmarkBorderRounded";
import SearchOffOutlined from "@mui/icons-material/SearchOffOutlined";
import { stories, type Comment, type Episode, type Home, type Show } from "./storyApi";
import { EpisodeCard, EpisodeRow, Hero, ShowCard, useEpisodeActions } from "./components";
import { Btn, C, Chip, Empty, ErrorNote, FONT_DISPLAY, Modal, Select, Shelf, ShelfSkeleton, Skeleton, TextArea, ago, errMsg, gradientFor, useToast } from "./ui";

function useLoad<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let live = true;
    setLoading(true);
    setError("");
    fn().then((d) => live && setData(d)).catch((e) => live && setError(errMsg(e))).finally(() => live && setLoading(false));
    return () => { live = false; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick]);
  return { data, setData, error, loading, reload: () => setTick((t) => t + 1) };
}

function PageSkeleton() {
  return (
    <Box data-testid="page-skeleton">
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "360px 1fr" }, gap: 5, mb: 5 }}>
        <Skeleton w="100%" h={330} r={22} />
        <Box><Skeleton h={30} w="60%" sx={{ mb: 2 }} />{[0, 1, 2, 3].map((i) => <Skeleton key={i} h={64} sx={{ mb: 1.2 }} r={12} />)}</Box>
      </Box>
      <ShelfSkeleton />
    </Box>
  );
}

// ---- comments -----------------------------------------------------------------------------

export function CommentsModal({ ep, onClose, canModerate }: { ep: Episode; onClose: () => void; canModerate: boolean }) {
  const toast = useToast();
  const { data, setData, error, loading, reload } = useLoad(() => stories.comments(ep.id), [ep.id]);
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const post = async () => {
    if (!body.trim()) return;
    setBusy(true);
    try {
      const c = await stories.addComment(ep.id, body);
      setData((l) => [...(l ?? []), c]);
      setBody("");
    } catch (e) { toast.show(errMsg(e), "error"); } finally { setBusy(false); }
  };
  const act = async (c: Comment, a: "report" | "hide" | "show" | "delete") => {
    try {
      await stories.commentAction(c.id, a, a === "report" ? "Reported from the episode page" : undefined);
      toast.show(a === "report" ? "Reported to the author and moderators" : a === "hide" ? "Comment hidden" : a === "show" ? "Comment restored" : "Comment deleted");
      reload();
    } catch (e) { toast.show(errMsg(e), "error"); }
  };
  return (
    <Modal title={`Comments · ${ep.title}`} onClose={onClose}>
      {loading && <Skeleton h={40} />}
      {error && <ErrorNote message={error} onRetry={reload} />}
      {data && data.length === 0 && <Box sx={{ color: C.muted, fontSize: 14, py: 2 }}>No comments yet. Be the first to say something.</Box>}
      {data?.map((c) => (
        <Box key={c.id} data-testid="comment" sx={{ py: 1.2, borderBottom: `1px solid ${C.line}`, opacity: c.status === "hidden" ? 0.55 : 1 }}>
          <Box sx={{ display: "flex", gap: 1, fontSize: 13, color: C.muted }}>
            <b style={{ color: C.ink }}>{c.author.name}</b> · {ago(c.created_at)}
            {c.status === "hidden" && <Box component="span" sx={{ background: C.chip, px: 1, borderRadius: 9 }}>hidden</Box>}
            {c.reports > 0 && <Box component="span" sx={{ background: C.dangerSoft, color: C.danger, px: 1, borderRadius: 9 }}>{c.reports} report{c.reports > 1 ? "s" : ""}</Box>}
          </Box>
          <Box sx={{ fontSize: 14.5, mt: 0.3, whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{c.body}</Box>
          <Box sx={{ display: "flex", gap: 1.5, mt: 0.5 }}>
            {!c.mine && !c.reported && <Box component="button" type="button" onClick={() => void act(c, "report")} sx={linkBtn}>Report</Box>}
            {c.reported && <span style={{ fontSize: 12.5, color: C.muted }}>Reported</span>}
            {canModerate && (c.status === "visible" ? <Box component="button" type="button" onClick={() => void act(c, "hide")} sx={linkBtn}>Hide</Box> : <Box component="button" type="button" onClick={() => void act(c, "show")} sx={linkBtn}>Show again</Box>)}
            {(c.mine || canModerate) && <Box component="button" type="button" onClick={() => void act(c, "delete")} sx={linkBtn}>Delete</Box>}
          </Box>
        </Box>
      ))}
      <Box sx={{ mt: 2 }}>
        <TextArea value={body} onChange={(e) => setBody(e.target.value)} placeholder="Add a comment" maxLength={2000} testId="comment-input" style={{ minHeight: 64 }} />
        <Box sx={{ display: "flex", justifyContent: "flex-end", mt: 1 }}><Btn variant="primary" onClick={() => void post()} disabled={busy || !body.trim()} testId="comment-post">Post comment</Btn></Box>
      </Box>
    </Modal>
  );
}
const linkBtn = { border: "none", background: "none", color: C.muted, fontSize: 12.5, cursor: "pointer", fontFamily: "inherit", textDecoration: "underline", p: 0 };

// ---- home -----------------------------------------------------------------------------------

function HeroFor({ ep, kicker }: { ep: Episode; kicker: string }) {
  const { data, setData } = useLoad(() => stories.show(ep.show_id), [ep.show_id]);
  if (!data) return <PageSkeleton />;
  return (
    <Hero
      show={data.show}
      episodes={data.episodes}
      kicker={kicker}
      setEpisodes={(f) => setData((d) => (d ? { ...d, episodes: f(d.episodes) } : d))}
    />
  );
}

const uniqShow = (l: Episode[]) => l.filter((e, i) => l.findIndex((x) => x.show_id === e.show_id) === i);

export function HomePage() {
  const nav = useNavigate();
  const { data, error, loading, reload } = useLoad<Home>(() => stories.home(), []);
  if (loading && !data) return <PageSkeleton />;
  if (error) return <ErrorNote message={error} onRetry={reload} />;
  if (!data) return null;
  const hero = data.continue[0] ?? data.new[0] ?? null;
  if (!hero) {
    return (
      <Empty
        icon={<AutoStoriesOutlined fontSize="inherit" />}
        title="No stories yet"
        hint="Colleagues can write stories here and listeners hear them read aloud. Be the first to share one."
        action={<Btn variant="primary" size="lg" onClick={() => nav("/podcasts/write?new=1")} testId="empty-write">Write a story</Btn>}
      />
    );
  }
  return (
    <Box>
      <HeroFor ep={hero} kicker={data.continue[0] ? "Pick up where you left off" : "New this week"} />
      {data.continue.length > 0 && (
        <Shelf title="Continue listening">{uniqShow(data.continue).map((e) => <EpisodeCard key={e.id} ep={e} />)}</Shelf>
      )}
      {data.new.length > 0 && <Shelf title="New this week">{uniqShow(data.new).map((e) => <EpisodeCard key={e.id} ep={e} />)}</Shelf>}
      {data.following.length > 0 && <Shelf title="From people and series you follow">{uniqShow(data.following).map((e) => <EpisodeCard key={e.id} ep={e} />)}</Shelf>}
      {data.trending.length > 0 && <Shelf title="Trending in the company">{data.trending.map((s) => <ShowCard key={s.id} show={s} />)}</Shelf>}
      {data.genres.length > 0 && (
        <Shelf title="Genres" action={<Box component="button" type="button" onClick={() => nav("/podcasts/shows")} sx={{ ...linkBtn, fontSize: 13.5 }}>Browse all</Box>}>
          {data.genres.map((g) => (
            <Box
              component="button"
              type="button"
              key={g.genre}
              data-testid="genre-tile"
              onClick={() => nav(`/podcasts/shows?genre=${encodeURIComponent(g.genre)}`)}
              sx={{ width: 170, height: 96, flexShrink: 0, border: "none", borderRadius: "16px", background: gradientFor(`${hash(g.genre) % 360},${(hash(g.genre) + 50) % 360},135`), color: "#fff", textAlign: "left", p: 1.6, cursor: "pointer", fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 17, display: "flex", flexDirection: "column", justifyContent: "flex-end", scrollSnapAlign: "start" }}
            >
              {g.genre}
              <Box component="span" sx={{ fontSize: 12, fontWeight: 600, opacity: 0.85, fontFamily: "Inter, sans-serif" }}>{g.count} {g.count === 1 ? "show" : "shows"}</Box>
            </Box>
          ))}
        </Shelf>
      )}
    </Box>
  );
}

function hash(s: string): number {
  let h = 0;
  for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return h;
}

// ---- browse shows -------------------------------------------------------------------------

export function ShowsPage() {
  const [params, setParams] = useSearchParams();
  const genre = params.get("genre") ?? "";
  const sort = params.get("sort") ?? "new";
  const { data: genres } = useLoad(() => stories.home().then((h) => h.genres), []);
  const { data, error, loading, reload } = useLoad(() => stories.shows({ genre, sort: sort === "new" ? "" : sort }), [genre, sort]);
  const set = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v); else p.delete(k);
    setParams(p);
  };
  return (
    <Box>
      <Box component="h1" sx={pageTitle}>Shows</Box>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center", mb: 3 }}>
        <Chip active={!genre} onClick={() => set("genre", "")} testId="genre-all">All</Chip>
        {genres?.map((g) => <Chip key={g.genre} active={genre === g.genre} onClick={() => set("genre", g.genre)}>{g.genre}</Chip>)}
        <Box sx={{ flex: 1 }} />
        <Box sx={{ width: 170 }}>
          <Select value={sort} onChange={(e) => set("sort", e.target.value)} aria-label="Sort shows">
            <option value="new">Recently updated</option>
            <option value="popular">Most listened</option>
            <option value="title">Title A–Z</option>
          </Select>
        </Box>
      </Box>
      {error && <ErrorNote message={error} onRetry={reload} />}
      {loading && !data && <ShelfSkeleton />}
      {data && data.length === 0 && <Empty icon={<HeadphonesOutlined fontSize="inherit" />} title={genre ? `No ${genre} shows yet` : "No shows yet"} hint="When colleagues publish stories they appear here." />}
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(170px, 1fr))", gap: 3 }}>
        {data?.map((s) => <Box key={s.id} sx={{ display: "flex" }}><ShowCard show={s} /></Box>)}
      </Box>
    </Box>
  );
}
const pageTitle = { m: 0, mb: 2, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: { xs: 26, md: 32 }, letterSpacing: "-0.03em" };

// ---- one show ------------------------------------------------------------------------------

export function ShowPage() {
  const { id = "" } = useParams();
  const { data, setData, error, loading, reload } = useLoad(() => stories.show(id), [id]);
  const [commentsFor, setCommentsFor] = useState<Episode | null>(null);
  if (loading && !data) return <PageSkeleton />;
  if (error) return <ErrorNote message={error === "not found" ? "This story does not exist or you do not have access to it." : error} onRetry={reload} />;
  if (!data) return null;
  return (
    <Box>
      <Hero show={data.show} episodes={data.episodes} setEpisodes={(f) => setData((d) => (d ? { ...d, episodes: f(d.episodes) } : d))} footer={data.show.comments_on && data.episodes.length > 0 ? (
        <Box sx={{ mt: 2 }}><Btn variant="ghost" size="sm" onClick={() => setCommentsFor(data.episodes.find((e) => e.status === "published") ?? data.episodes[0])} testId="open-comments">Comments</Btn></Box>
      ) : undefined} />
      {commentsFor && <CommentsModal ep={commentsFor} onClose={() => setCommentsFor(null)} canModerate={data.show.mine} />}
    </Box>
  );
}

/** An episode link (notification target): opens the show page and starts nothing. */
export function EpisodeRedirect() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const [err, setErr] = useState("");
  useEffect(() => {
    stories.episode(id).then((e) => nav(`/podcasts/show/${e.show_id}`, { replace: true })).catch((e) => setErr(errMsg(e)));
  }, [id, nav]);
  return err ? <ErrorNote message={err === "not found" ? "This episode is not available." : err} /> : <PageSkeleton />;
}

// ---- library ---------------------------------------------------------------------------------

export function LibraryPage() {
  const [tab, setTab] = useState<"saved" | "shows" | "liked" | "history">("saved");
  const { data, setData, error, loading, reload } = useLoad(() => stories.library(), []);
  const setList = useCallback((key: "episodes" | "liked" | "history") => (f: (l: Episode[]) => Episode[]) => setData((d) => (d ? { ...d, [key]: f(d[key]) } : d)), [setData]);
  const aSaved = useEpisodeActions(setList("episodes"));
  const aLiked = useEpisodeActions(setList("liked"));
  const aHist = useEpisodeActions(setList("history"));
  const nav = useNavigate();
  const list = tab === "saved" ? data?.episodes : tab === "liked" ? data?.liked : tab === "history" ? data?.history : null;
  const act = tab === "saved" ? aSaved : tab === "liked" ? aLiked : aHist;
  return (
    <Box>
      <Box component="h1" sx={pageTitle}>Your library</Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2.5, flexWrap: "wrap" }}>
        <Chip active={tab === "saved"} onClick={() => setTab("saved")} testId="lib-saved">Saved episodes{data ? ` · ${data.episodes.length}` : ""}</Chip>
        <Chip active={tab === "shows"} onClick={() => setTab("shows")} testId="lib-shows">Saved shows{data ? ` · ${data.shows.length}` : ""}</Chip>
        <Chip active={tab === "liked"} onClick={() => setTab("liked")}>Liked{data ? ` · ${data.liked.length}` : ""}</Chip>
        <Chip active={tab === "history"} onClick={() => setTab("history")}>History</Chip>
      </Box>
      {error && <ErrorNote message={error} onRetry={reload} />}
      {loading && !data && <Skeleton h={80} />}
      {data && tab === "shows" && (data.shows.length === 0
        ? <Empty icon={<BookmarkBorderRounded fontSize="inherit" />} title="No saved shows" hint="Use Save on a show to keep it here." action={<Btn variant="soft" onClick={() => nav("/podcasts/shows")}>Browse shows</Btn>} />
        : <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(170px, 1fr))", gap: 3 }}>{data.shows.map((s) => <ShowCard key={s.id} show={s} />)}</Box>)}
      {data && tab !== "shows" && list && (list.length === 0
        ? <Empty icon={<BookmarkBorderRounded fontSize="inherit" />} title={tab === "saved" ? "Nothing saved yet" : tab === "liked" ? "No liked episodes yet" : "Nothing played yet"} hint={tab === "saved" ? "Tap the bookmark on an episode to keep it here." : "Episodes you listen to show up here."} action={<Btn variant="soft" onClick={() => nav("/podcasts")}>Find something to listen to</Btn>} />
        : <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "20px", p: { xs: 1.5, md: 2.5 } }}>{list.map((e, i) => <EpisodeRow key={e.id} ep={e} index={i + 1} actions={act} />)}</Box>)}
    </Box>
  );
}

// ---- search -------------------------------------------------------------------------------------

export function SearchPage() {
  const [params] = useSearchParams();
  const q = params.get("q") ?? "";
  const { data, error, loading, reload } = useLoad(() => (q ? stories.search(q) : Promise.resolve({ shows: [], episodes: [] })), [q]);
  const [eps, setEps] = useState<Episode[] | null>(null);
  useEffect(() => setEps(data?.episodes ?? null), [data]);
  const actions = useEpisodeActions((f) => setEps((l) => (l ? f(l) : l)));
  if (!q) return <Empty icon={<SearchOffOutlined fontSize="inherit" />} title="Search stories" hint="Type a title, an author or a few words from a story." />;
  return (
    <Box>
      <Box component="h1" sx={pageTitle}>Results for “{q}”</Box>
      {error && <ErrorNote message={error} onRetry={reload} />}
      {loading && <ShelfSkeleton />}
      {data && data.shows.length === 0 && (eps ?? []).length === 0 && !loading && <Empty icon={<SearchOffOutlined fontSize="inherit" />} title="Nothing found" hint="Try a different word, or check the spelling." />}
      {data && data.shows.length > 0 && <Shelf title="Shows">{data.shows.map((s) => <ShowCard key={s.id} show={s} />)}</Shelf>}
      {eps && eps.length > 0 && (
        <Box>
          <Box component="h2" sx={{ m: 0, mb: 1, fontFamily: FONT_DISPLAY, fontSize: 20, fontWeight: 800 }}>Episodes</Box>
          <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "20px", p: { xs: 1.5, md: 2.5 } }}>
            {eps.map((e, i) => <EpisodeRow key={e.id} ep={e} index={i + 1} actions={actions} />)}
          </Box>
        </Box>
      )}
    </Box>
  );
}

export type { Show };
