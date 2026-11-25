import { useEffect, useMemo, useRef, useState } from "react";
import { Box, Typography, IconButton, Slider, Button, Modal, ModalDialog, ModalClose, Textarea, DialogTitle, Sheet } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import PauseRoundedIcon from "@mui/icons-material/PauseRounded";
import SkipNextRoundedIcon from "@mui/icons-material/SkipNextRounded";
import SkipPreviousRoundedIcon from "@mui/icons-material/SkipPreviousRounded";
import ShuffleRoundedIcon from "@mui/icons-material/ShuffleRounded";
import RepeatRoundedIcon from "@mui/icons-material/RepeatRounded";
import RepeatOneRoundedIcon from "@mui/icons-material/RepeatOneRounded";
import FavoriteIcon from "@mui/icons-material/Favorite";
import FavoriteBorderIcon from "@mui/icons-material/FavoriteBorder";
import LyricsRoundedIcon from "@mui/icons-material/LyricsRounded";
import KeyboardArrowDownRoundedIcon from "@mui/icons-material/KeyboardArrowDownRounded";
import { useNavigate } from "react-router-dom";
import { usePlayer } from "./player";
import { useLibrary } from "./library";
import { Cover, coverColors, EmptyNote } from "./ui";
import { formatClock } from "./media";
import { getLyrics, setLyrics } from "./api";
import { activeLine, parseLRC, type LyricLine } from "./logic";
import { albumKey } from "./logic";

/** NowPlaying is the full-page player: big cover, controls and synced lyrics. */
export function NowPlaying() {
  const p = usePlayer();
  const lib = useLibrary();
  const nav = useNavigate();
  const t = p.current;
  const [lyrics, setLyricsText] = useState<{ id: string; text: string } | null>(null);
  const [editing, setEditing] = useState(false);
  const [loadErr, setLoadErr] = useState(false);

  useEffect(() => {
    setLyricsText(null);
    setLoadErr(false);
    if (!t?.has_lyrics) return;
    let cancelled = false;
    getLyrics(t.id)
      .then((r) => !cancelled && setLyricsText({ id: t.id, text: r.lyrics }))
      .catch(() => !cancelled && setLoadErr(true));
    return () => {
      cancelled = true;
    };
  }, [t?.id, t?.has_lyrics]);

  const parsed = useMemo(() => (lyrics && t && lyrics.id === t.id ? parseLRC(lyrics.text) : null), [lyrics, t]);
  const active = parsed?.timed ? activeLine(parsed.lines, p.position + 0.15) : -1;
  const lineRefs = useRef<(HTMLDivElement | null)[]>([]);
  const scroller = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (active < 0) return;
    const el = lineRefs.current[active];
    const box = scroller.current;
    if (el && box) box.scrollTo({ top: el.offsetTop - box.clientHeight / 2 + el.clientHeight / 2, behavior: "smooth" });
  }, [active]);

  if (!t) {
    return (
      <EmptyNote title="Nothing is playing" text="Pick a song from Home or your Library and it will show up here with its lyrics." action={<Button onClick={() => nav("/music")}>Browse music</Button>} />
    );
  }
  const [c1, c2] = coverColors(t.album || t.title);
  const dur = p.duration || t.duration_seconds || 0;
  const RepeatIcon = p.repeat === "one" ? RepeatOneRoundedIcon : RepeatRoundedIcon;
  const mine = lib.userId === t.owner_id;

  return (
    <Box
      data-testid="now-playing"
      sx={{
        mx: { xs: -1.5, md: -3 },
        mt: { xs: -1.5, md: -3 },
        p: { xs: 2, md: 4 },
        minHeight: { md: "calc(100vh - var(--ad-topbar-h, 56px) - 80px)" },
        background: `linear-gradient(160deg, ${c1.replace("72%", "92%")} 0%, #fff 55%)`,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
        <IconButton variant="soft" color="neutral" size="sm" aria-label="Close now playing" onClick={() => nav(-1)} sx={{ borderRadius: "50%" }}>
          <KeyboardArrowDownRoundedIcon />
        </IconButton>
        <Typography level="body-xs" sx={{ ml: 1.5, textTransform: "uppercase", letterSpacing: ".08em", fontWeight: 700, color: "text.tertiary" }}>
          Now playing
        </Typography>
      </Box>
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "minmax(280px, 440px) 1fr" }, gap: { xs: 3, md: 6 }, alignItems: "start" }}>
        <Box sx={{ maxWidth: { xs: 440, md: "min(440px, calc(100vh - 580px))" }, minWidth: { md: 280 }, mx: { xs: "auto", md: 0 }, width: "100%" }}>
          <Box sx={{ borderRadius: "20px", overflow: "hidden", boxShadow: `0 24px 60px -20px ${c2}`, lineHeight: 0, aspectRatio: "1", "& > div": { width: "100% !important", height: "100% !important" } }}>
            <Cover url={t.art_url} name={t.album || t.title} size="100%" radius={20} />
          </Box>
          <Box sx={{ mt: 3, display: "flex", alignItems: "flex-start", gap: 1 }}>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography level="h2" noWrap sx={{ fontSize: "xl2" }} data-testid="np-title">
                {t.title}
              </Typography>
              <Typography level="body-md" noWrap sx={{ color: "text.secondary", cursor: "pointer", "&:hover": { textDecoration: "underline" } }} onClick={() => nav(`/music/artist/${encodeURIComponent(t.artist || "Unknown artist")}`)}>
                {t.artist || "Unknown artist"}
              </Typography>
              <Typography level="body-sm" noWrap sx={{ color: "text.tertiary", cursor: "pointer" }} onClick={() => nav(`/music/album/${encodeURIComponent(albumKey(t))}`)}>
                {t.album}
                {t.year ? ` · ${t.year}` : ""}
              </Typography>
            </Box>
            <IconButton variant="plain" color={t.liked ? "danger" : "neutral"} aria-label={t.liked ? "Remove like" : "Like"} aria-pressed={!!t.liked} onClick={() => lib.actions.toggleLike(t)}>
              {t.liked ? <FavoriteIcon /> : <FavoriteBorderIcon />}
            </IconButton>
          </Box>
          <Slider aria-label="Seek" value={Math.min(p.position, dur)} min={0} max={dur || 1} step={0.5} onChange={(_, v) => p.seek(v as number)} sx={{ mt: 2, "--Slider-trackSize": "5px" }} />
          <Box sx={{ display: "flex", justifyContent: "space-between", mt: -0.5 }}>
            <Typography level="body-xs" sx={{ fontVariantNumeric: "tabular-nums", color: "text.tertiary" }}>
              {formatClock(p.position)}
            </Typography>
            <Typography level="body-xs" sx={{ fontVariantNumeric: "tabular-nums", color: "text.tertiary" }}>
              {formatClock(dur)}
            </Typography>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 1.5, mt: 1 }}>
            <IconButton variant="plain" color={p.shuffle ? "primary" : "neutral"} aria-pressed={p.shuffle} aria-label="Shuffle" onClick={p.toggleShuffle}>
              <ShuffleRoundedIcon />
            </IconButton>
            <IconButton variant="plain" color="neutral" size="lg" aria-label="Previous" onClick={p.prev}>
              <SkipPreviousRoundedIcon />
            </IconButton>
            <IconButton aria-label={p.playing ? "Pause" : "Play"} onClick={p.toggle} sx={{ width: 64, height: 64, borderRadius: "50%", bgcolor: "primary.solidBg", color: "#fff", "&:hover": { bgcolor: "primary.solidHoverBg" } }}>
              {p.playing ? <PauseRoundedIcon sx={{ fontSize: 34 }} /> : <PlayArrowRoundedIcon sx={{ fontSize: 34 }} />}
            </IconButton>
            <IconButton variant="plain" color="neutral" size="lg" aria-label="Next" onClick={p.next}>
              <SkipNextRoundedIcon />
            </IconButton>
            <IconButton variant="plain" color={p.repeat !== "off" ? "primary" : "neutral"} aria-label={`Repeat ${p.repeat}`} onClick={p.cycleRepeat}>
              <RepeatIcon />
            </IconButton>
          </Box>
        </Box>

        {/* Lyrics */}
        <Sheet variant="outlined" sx={{ borderRadius: "20px", p: { xs: 2, md: 3 }, minHeight: { xs: 260, md: 520 }, maxHeight: { md: "calc(100vh - 220px)" }, display: "flex", flexDirection: "column", bgcolor: "rgba(255,255,255,.7)" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
            <LyricsRoundedIcon sx={{ color: "text.tertiary" }} />
            <Typography level="title-md" sx={{ flex: 1 }}>
              Lyrics
            </Typography>
            {parsed?.timed && (
              <Typography level="body-xs" sx={{ color: "text.tertiary" }}>
                Synced · click a line to jump
              </Typography>
            )}
            {mine && (
              <Button size="sm" variant="soft" color="neutral" onClick={() => setEditing(true)}>
                {t.has_lyrics ? "Edit" : "Add lyrics"}
              </Button>
            )}
          </Box>
          <Box ref={scroller} data-testid="lyrics" sx={{ flex: 1, overflowY: "auto", position: "relative", pr: 1 }}>
            {!t.has_lyrics && (
              <Box sx={{ py: 6, textAlign: "center", color: "text.tertiary" }}>
                <Typography level="title-sm">No lyrics for this song</Typography>
                <Typography level="body-sm" sx={{ mt: 0.5 }}>
                  Upload a .lrc file with the song (same file name) to show synced lyrics here.
                </Typography>
              </Box>
            )}
            {t.has_lyrics && !parsed && !loadErr && <Typography level="body-sm">Loading lyrics…</Typography>}
            {loadErr && <Typography level="body-sm" color="danger">Could not load the lyrics.</Typography>}
            {parsed &&
              parsed.lines.map((l: LyricLine, i: number) => (
                <Box
                  key={i}
                  ref={(el: HTMLDivElement | null) => {
                    lineRefs.current[i] = el;
                  }}
                  data-active={i === active || undefined}
                  onClick={() => parsed.timed && p.seek(l.time)}
                  sx={{
                    py: 0.75,
                    fontFamily: "display",
                    fontSize: parsed.timed ? { xs: "xl", md: "xl2" } : "lg",
                    fontWeight: 700,
                    lineHeight: 1.25,
                    cursor: parsed.timed ? "pointer" : "default",
                    color: !parsed.timed ? "text.primary" : i === active ? "primary.solidBg" : i < active ? "text.tertiary" : "text.secondary",
                    opacity: !parsed.timed || i === active ? 1 : 0.62,
                    transition: "color .25s, opacity .25s, transform .25s",
                    transform: i === active ? "translateX(4px)" : "none",
                    "&:hover": { opacity: 1 },
                  }}
                >
                  {l.text || "♪"}
                </Box>
              ))}
          </Box>
        </Sheet>
      </Box>
      {editing && t && (
        <LyricsDialog
          initial={lyrics?.id === t.id ? lyrics.text : ""}
          onClose={() => setEditing(false)}
          onSave={async (text) => {
            await setLyrics(t.id, text);
            lib.patch(t.id, { has_lyrics: text.trim() !== "" });
            setLyricsText({ id: t.id, text });
            setEditing(false);
          }}
        />
      )}
    </Box>
  );
}

function LyricsDialog({ initial, onSave, onClose }: { initial: string; onSave: (t: string) => Promise<void>; onClose: () => void }) {
  const [text, setText] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "100vw", sm: 560 }, maxWidth: "100vw" }}>
        <ModalClose />
        <DialogTitle>Lyrics</DialogTitle>
        <Typography level="body-sm" sx={{ color: "text.tertiary" }}>
          Paste plain text, or timed lines like [01:23.40] words (a .lrc file).
        </Typography>
        <Textarea minRows={10} maxRows={16} value={text} onChange={(e) => setText(e.target.value)} sx={{ mt: 1, fontFamily: "monospace" }} slotProps={{ textarea: { "aria-label": "Lyrics text" } }} />
        {err && <Typography color="danger" level="body-sm">{err}</Typography>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await onSave(text);
              } catch (e) {
                setErr((e as Error).message);
                setBusy(false);
              }
            }}
          >
            Save
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
