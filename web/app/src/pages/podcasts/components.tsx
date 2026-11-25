import { useEffect, useState, type ReactNode } from "react";
import { Box } from "@mui/joy";
import { useNavigate } from "react-router-dom";
import PlayArrowRounded from "@mui/icons-material/PlayArrowRounded";
import PauseRounded from "@mui/icons-material/PauseRounded";
import FavoriteBorderRounded from "@mui/icons-material/FavoriteBorderRounded";
import FavoriteRounded from "@mui/icons-material/FavoriteRounded";
import BookmarkBorderRounded from "@mui/icons-material/BookmarkBorderRounded";
import BookmarkRounded from "@mui/icons-material/BookmarkRounded";
import ExpandMoreRounded from "@mui/icons-material/ExpandMoreRounded";
import CheckCircleRounded from "@mui/icons-material/CheckCircleRounded";
import ChatBubbleOutlineRounded from "@mui/icons-material/ChatBubbleOutlineRounded";
import { stories, type Episode, type EpisodeDetail, type Show } from "./storyApi";
import { fmtLength, fmtTime, progressFraction, resumePosition } from "./playerLogic";
import { StoryText, usePlayer } from "./player";
import { Btn, C, Cover, FONT_DISPLAY, IconBtn, Skeleton, ago, errMsg, useToast } from "./ui";

// ---- cards for shelves -------------------------------------------------------------

export function ShowCard({ show, onOpen }: { show: Show; onOpen?: () => void }) {
  const nav = useNavigate();
  return (
    <Box
      component="button"
      type="button"
      data-testid="show-card"
      onClick={onOpen ?? (() => nav(`/podcasts/show/${show.id}`))}
      sx={{ width: 170, flexShrink: 0, textAlign: "left", border: "none", background: "transparent", p: 0, cursor: "pointer", fontFamily: "inherit", color: C.ink, scrollSnapAlign: "start", "&:hover img, &:hover [role=img]": { transform: "translateY(-2px)" } }}
    >
      <Cover title={show.title} url={show.cover_url} style={show.cover_style} size={170} radius={16} sx={{ transition: "transform .15s" }} />
      <Box sx={{ mt: 1, fontWeight: 700, fontSize: 14.5, lineHeight: 1.25, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{show.title}</Box>
      <Box sx={{ fontSize: 12.5, color: C.muted, mt: 0.2 }}>{show.author.name} · {show.published} {show.published === 1 ? "episode" : "episodes"}</Box>
    </Box>
  );
}

export function EpisodeCard({ ep }: { ep: Episode }) {
  const p = usePlayer();
  const nav = useNavigate();
  const frac = progressFraction(ep.position_ms, ep.progress_duration_ms || ep.duration_ms, ep.completed);
  const isCur = p.ep?.id === ep.id;
  return (
    <Box data-testid="episode-card" sx={{ width: 170, flexShrink: 0, scrollSnapAlign: "start" }}>
      <Box sx={{ position: "relative" }}>
        <Box component="button" type="button" onClick={() => nav(`/podcasts/show/${ep.show_id}`)} sx={{ border: "none", p: 0, background: "none", cursor: "pointer", display: "block" }} aria-label={`Open ${ep.show_title}`}>
          <Cover title={ep.show_title} url={ep.cover_url} style={ep.cover_style} size={170} radius={16} />
        </Box>
        <IconBtn
          label={isCur && p.playing ? `Pause ${ep.title}` : `Play ${ep.title}`}
          onClick={() => (isCur ? p.toggle() : void p.play(ep.id))}
          size={40}
          sx={{ position: "absolute", right: 8, top: 8, background: "rgba(255,255,255,.95)", color: C.accentDark, boxShadow: "0 2px 8px rgba(0,0,0,.25)", "&:hover:not(:disabled)": { background: "#fff" } }}
        >
          {isCur && p.playing ? <PauseRounded /> : <PlayArrowRounded />}
        </IconBtn>
        {frac > 0 && (
          <Box sx={{ position: "absolute", left: 10, right: 10, bottom: 4, height: 3, background: "rgba(255,255,255,.55)", borderRadius: 2 }}>
            <Box sx={{ width: `${frac * 100}%`, height: "100%", background: "#fff", borderRadius: 2 }} />
          </Box>
        )}
      </Box>
      <Box sx={{ mt: 1, fontWeight: 700, fontSize: 14, lineHeight: 1.25, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{ep.title}</Box>
      <Box sx={{ fontSize: 12.5, color: C.muted, mt: 0.2 }}>{ep.show_title} · {fmtLength(ep.duration_ms)}</Box>
    </Box>
  );
}

// ---- episode details (read along) ----------------------------------------------------

const detailCache = new Map<string, EpisodeDetail>();

export function useEpisodeDetail(id: string, enabled: boolean) {
  const [d, setD] = useState<EpisodeDetail | null>(detailCache.get(id) ?? null);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (!enabled) return;
    let live = true;
    setErr("");
    stories.episode(id).then((x) => { detailCache.set(id, x); if (live) setD(x); }).catch((e) => live && setErr(errMsg(e)));
    return () => { live = false; };
  }, [id, enabled]);
  return { d, err };
}

/** The "Read along / Story text" panel of an episode: summary, show notes and the story text. */
export function ReadAlongInline({ ep }: { ep: Episode }) {
  const { d, err } = useEpisodeDetail(ep.id, true);
  const p = usePlayer();
  const isCur = p.ep?.id === ep.id;
  if (err) return <Box sx={{ color: C.danger, fontSize: 14, py: 1 }}>{err}</Box>;
  if (!d) return <Box sx={{ py: 1.5 }}><Skeleton h={14} sx={{ mb: 1 }} /><Skeleton h={14} w="90%" sx={{ mb: 1 }} /><Skeleton h={14} w="70%" /></Box>;
  return (
    <Box data-testid="readalong-panel" sx={{ background: "#FBFAF6", border: `1px solid ${C.line}`, borderRadius: "14px", p: 2, mt: 1 }}>
      {(d.summary || d.show_description) && (
        <Box sx={{ mb: 1.5, pb: 1.5, borderBottom: `1px dashed ${C.line}` }}>
          <Box sx={{ fontSize: 11.5, fontWeight: 800, letterSpacing: ".08em", textTransform: "uppercase", color: C.muted, mb: 0.5 }}>Show notes</Box>
          {d.summary && <Box sx={{ fontSize: 14.5, mb: 0.6 }}>{d.summary}</Box>}
          {d.show_description && <Box sx={{ fontSize: 13.5, color: C.muted }}>{d.show_description}</Box>}
        </Box>
      )}
      <Box sx={{ fontSize: 11.5, fontWeight: 800, letterSpacing: ".08em", textTransform: "uppercase", color: C.muted, mb: 0.8 }}>Story text</Box>
      <StoryText ep={d} current={isCur ? p.sentence : -1} maxHeight={320} onPick={(i) => (isCur ? p.seekSentence(i) : void p.play(ep.id, { sentence: i }))} />
    </Box>
  );
}

// ---- episode row ----------------------------------------------------------------------

export function useEpisodeActions(setList: (f: (l: Episode[]) => Episode[]) => void) {
  const toast = useToast();
  const patch = (id: string, f: Partial<Episode> | ((e: Episode) => Partial<Episode>)) =>
    setList((l) => l.map((e) => (e.id === id ? { ...e, ...(typeof f === "function" ? f(e) : f) } : e)));
  return {
    like: async (e: Episode) => {
      patch(e.id, (x) => ({ liked: !x.liked, likes: x.likes + (x.liked ? -1 : 1) }));
      try { await stories.like(e.id, !e.liked); } catch (err) { patch(e.id, { liked: e.liked, likes: e.likes }); toast.show(errMsg(err), "error"); }
    },
    save: async (e: Episode) => {
      patch(e.id, { saved: !e.saved });
      try { await stories.save("episode", e.id, !e.saved); toast.show(e.saved ? "Removed from your library" : "Saved to your library"); } catch (err) { patch(e.id, { saved: e.saved }); toast.show(errMsg(err), "error"); }
    },
    played: async (e: Episode) => {
      patch(e.id, { completed: !e.completed, position_ms: 0 });
      try { await stories.markPlayed(e.id, !e.completed); } catch (err) { patch(e.id, { completed: e.completed, position_ms: e.position_ms }); toast.show(errMsg(err), "error"); }
    },
  };
}

export function EpisodeRow({ ep, index, actions, onOpenComments }: { ep: Episode; index: number; actions: ReturnType<typeof useEpisodeActions>; onOpenComments?: () => void }) {
  const p = usePlayer();
  const [open, setOpen] = useState(false);
  const isCur = p.ep?.id === ep.id;
  const dur = ep.progress_duration_ms || ep.duration_ms;
  const frac = isCur && p.durationMs > 0 ? p.positionMs / p.durationMs : progressFraction(ep.position_ms, dur, ep.completed);
  const resume = resumePosition(ep.position_ms, dur, ep.completed);
  return (
    <Box data-testid="episode-row" data-episode-id={ep.id} sx={{ py: 1.4, borderBottom: `1px solid ${C.line}`, position: "relative" }}>
      <Box sx={{ display: "flex", gap: 1.4, alignItems: "flex-start" }}>
        <IconBtn
          label={isCur && p.playing ? `Pause ${ep.title}` : `Play ${ep.title}`}
          onClick={() => (isCur ? p.toggle() : void p.play(ep.id))}
          size={40}
          testId="row-play"
          sx={{ background: isCur ? C.accent : C.accentSoft, color: isCur ? "#fff" : C.accentDark, "&:hover:not(:disabled)": { background: isCur ? C.accentDark : "#D3E8E2" }, mt: 0.2 }}
        >
          {isCur && p.playing ? <PauseRounded /> : <PlayArrowRounded />}
        </IconBtn>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box sx={{ display: "flex", gap: 1, alignItems: "baseline" }}>
            <Box sx={{ color: C.faint, fontSize: 13, fontWeight: 700, minWidth: 18 }}>{index}</Box>
            <Box sx={{ fontWeight: 700, fontSize: 15, lineHeight: 1.3, color: C.ink, flex: 1, minWidth: 0 }}>{ep.title}</Box>
            {ep.completed && <CheckCircleRounded sx={{ fontSize: 17, color: C.accent }} titleAccess="Played" />}
          </Box>
          {ep.summary && <Box sx={{ fontSize: 13.5, color: C.muted, mt: 0.3, display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{ep.summary}</Box>}
          <Box sx={{ display: "flex", gap: 1.2, flexWrap: "wrap", alignItems: "center", mt: 0.6, fontSize: 12.5, color: C.muted }}>
            <span>{fmtLength(ep.duration_ms)}</span>
            {ep.published_at && <span>· {ago(ep.published_at)}</span>}
            {!ep.completed && resume > 0 && <span style={{ color: C.accent, fontWeight: 700 }}>· Resume at {fmtTime(resume)}</span>}
            {ep.status !== "published" && <Box component="span" sx={{ background: C.chip, borderRadius: "999px", px: 1, fontWeight: 700, textTransform: "capitalize" }}>{ep.status}</Box>}
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.4, mt: 0.8, flexWrap: "wrap" }}>
            <Box
              component="button"
              type="button"
              aria-expanded={open}
              data-testid="readalong-expand"
              onClick={() => setOpen(!open)}
              sx={{ display: "inline-flex", alignItems: "center", gap: 0.3, border: `1px solid ${C.line}`, background: open ? C.accentSoft : C.surface, color: open ? C.accentDark : C.ink, borderRadius: "999px", px: 1.2, py: 0.35, fontSize: 12.5, fontWeight: 650, cursor: "pointer", fontFamily: "inherit" }}
            >
              Read along / Story text
              <ExpandMoreRounded sx={{ fontSize: 18, transform: open ? "rotate(180deg)" : "none", transition: "transform .15s" }} />
            </Box>
            <IconBtn label={ep.liked ? "Unlike" : "Like"} onClick={() => void actions.like(ep)} size={32} active={ep.liked} testId="like">
              {ep.liked ? <FavoriteRounded sx={{ fontSize: 19 }} /> : <FavoriteBorderRounded sx={{ fontSize: 19 }} />}
            </IconBtn>
            <Box sx={{ fontSize: 12.5, color: C.muted, mr: 0.6 }}>{ep.likes > 0 ? ep.likes : ""}</Box>
            <IconBtn label={ep.saved ? "Remove from library" : "Save to library"} onClick={() => void actions.save(ep)} size={32} active={ep.saved} testId="save">
              {ep.saved ? <BookmarkRounded sx={{ fontSize: 19 }} /> : <BookmarkBorderRounded sx={{ fontSize: 19 }} />}
            </IconBtn>
            {onOpenComments && (
              <Box component="button" type="button" onClick={onOpenComments} sx={{ display: "inline-flex", alignItems: "center", gap: 0.4, border: "none", background: "none", color: C.muted, fontSize: 12.5, cursor: "pointer", fontFamily: "inherit", px: 0.8 }}>
                <ChatBubbleOutlineRounded sx={{ fontSize: 17 }} /> {ep.comments}
              </Box>
            )}
            <Box component="button" type="button" onClick={() => void actions.played(ep)} sx={{ border: "none", background: "none", color: C.muted, fontSize: 12.5, cursor: "pointer", fontFamily: "inherit", textDecoration: "underline" }}>
              {ep.completed ? "Mark unplayed" : "Mark played"}
            </Box>
          </Box>
        </Box>
      </Box>
      {frac > 0 && <Box sx={{ position: "absolute", left: 0, right: 0, bottom: -1, height: 2, background: "transparent" }}><Box data-testid="row-progress" sx={{ width: `${Math.min(1, frac) * 100}%`, height: "100%", background: C.accent }} /></Box>}
      {open && <ReadAlongInline ep={ep} />}
    </Box>
  );
}

// ---- hero ---------------------------------------------------------------------------------

/** Big cover, title and a Resume/Play button on the left; the episode list on the right. */
export function Hero({ show, episodes, setEpisodes, kicker, footer }: { show: Show; episodes: Episode[]; setEpisodes: (f: (l: Episode[]) => Episode[]) => void; kicker?: string; footer?: ReactNode }) {
  const p = usePlayer();
  const toast = useToast();
  const nav = useNavigate();
  const actions = useEpisodeActions(setEpisodes);
  const [following, setFollowing] = useState(show.following);
  const [inLib, setInLib] = useState(show.in_library);
  useEffect(() => { setFollowing(show.following); setInLib(show.in_library); }, [show.id, show.following, show.in_library]);
  const published = episodes.filter((e) => e.status === "published");
  const inProgress = published.find((e) => !e.completed && e.position_ms > 0);
  const firstUnplayed = published.find((e) => !e.completed) ?? published[0];
  const target = (p.ep && p.ep.show_id === show.id ? published.find((e) => e.id === p.ep!.id) : undefined) ?? inProgress ?? firstUnplayed;
  const playingHere = !!target && p.ep?.id === target.id && p.playing;
  const label = playingHere ? "Pause" : target && target.id === inProgress?.id ? "Resume" : "Play";
  const toggleFollow = async () => {
    const on = !following;
    setFollowing(on);
    try { await stories.follow("show", show.id, on); toast.show(on ? "You will be notified of new episodes" : "Unfollowed"); } catch (e) { setFollowing(!on); toast.show(errMsg(e), "error"); }
  };
  const toggleLib = async () => {
    const on = !inLib;
    setInLib(on);
    try { await stories.save("show", show.id, on); toast.show(on ? "Saved to your library" : "Removed from your library"); } catch (e) { setInLib(!on); toast.show(errMsg(e), "error"); }
  };
  return (
    <Box data-testid="hero" sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "minmax(320px, 400px) minmax(0, 1fr)" }, gap: { xs: 2.5, md: 5 }, alignItems: "start", mb: 5 }}>
      <Box sx={{ minWidth: 0 }}>
        <Cover title={show.title} url={show.cover_url} style={show.cover_style} size="100%" radius={22} sx={{ maxWidth: { xs: 260, md: 330 }, mx: { xs: "auto", md: 0 }, mb: 2.5 }} />
        {kicker && <Box sx={{ fontSize: 12, fontWeight: 800, letterSpacing: ".1em", textTransform: "uppercase", color: C.accent, mb: 0.8 }}>{kicker}</Box>}
        <Box component="h1" sx={{ m: 0, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: { xs: 28, md: 36 }, lineHeight: 1.08, letterSpacing: "-0.03em", color: C.ink }} data-testid="hero-title">{show.title}</Box>
        <Box sx={{ mt: 1, color: C.muted, fontSize: 14.5 }}>
          By <Box component="span" sx={{ color: C.ink, fontWeight: 650 }}>{show.author.name}</Box> · {show.genre} · {show.published} {show.published === 1 ? "episode" : "episodes"} · {fmtLength(show.total_ms)}
        </Box>
        {show.description && <Box sx={{ mt: 1.2, fontSize: 14.5, lineHeight: 1.55, color: C.ink, display: "-webkit-box", WebkitLineClamp: 4, WebkitBoxOrient: "vertical", overflow: "hidden" }}>{show.description}</Box>}
        <Box sx={{ display: "flex", gap: 1.2, mt: 2.2, flexWrap: "wrap", alignItems: "center" }}>
          <Btn variant="primary" size="lg" testId="hero-play" disabled={!target} startIcon={playingHere ? <PauseRounded /> : <PlayArrowRounded />} onClick={() => target && (p.ep?.id === target.id ? p.toggle() : void p.play(target.id))}>
            {label}
          </Btn>
          {!show.mine && <Btn variant={following ? "soft" : "outline"} onClick={() => void toggleFollow()} testId="follow">{following ? "Following" : "Follow"}</Btn>}
          <Btn variant={inLib ? "soft" : "outline"} onClick={() => void toggleLib()} testId="save-show" startIcon={inLib ? <BookmarkRounded sx={{ fontSize: 18 }} /> : <BookmarkBorderRounded sx={{ fontSize: 18 }} />}>
            {inLib ? "In library" : "Save"}
          </Btn>
          {show.mine && <Btn variant="outline" onClick={() => nav(`/podcasts/write/${show.id}`)}>Edit story</Btn>}
        </Box>
        {footer}
      </Box>
      <Box sx={{ background: C.surface, borderRadius: "20px", border: `1px solid ${C.line}`, p: { xs: 1.5, md: 2.5 }, boxShadow: "0 1px 3px rgba(22,33,30,.05)", minWidth: 0 }} data-testid="episode-list">
        <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 17, mb: 0.5 }}>Episodes</Box>
        {episodes.length === 0 ? (
          <Box sx={{ color: C.muted, py: 3, fontSize: 14 }}>No episodes yet.</Box>
        ) : (
          episodes.map((e, i) => <EpisodeRow key={e.id} ep={e} index={i + 1} actions={actions} />)
        )}
      </Box>
    </Box>
  );
}
