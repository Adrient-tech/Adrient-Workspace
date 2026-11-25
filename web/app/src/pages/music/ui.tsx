import { useRef, type ReactNode } from "react";
import { Box, Typography, IconButton, Dropdown, Menu, MenuButton, MenuItem, ListDivider, Chip } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import PauseRoundedIcon from "@mui/icons-material/PauseRounded";
import MusicNoteRoundedIcon from "@mui/icons-material/MusicNoteRounded";
import MoreHorizRoundedIcon from "@mui/icons-material/MoreHorizRounded";
import FavoriteIcon from "@mui/icons-material/Favorite";
import FavoriteBorderIcon from "@mui/icons-material/FavoriteBorder";
import ChevronLeftRoundedIcon from "@mui/icons-material/ChevronLeftRounded";
import ChevronRightRoundedIcon from "@mui/icons-material/ChevronRightRounded";
import PlaylistAddIcon from "@mui/icons-material/PlaylistAdd";
import QueuePlayNextIcon from "@mui/icons-material/QueuePlayNext";
import AddToQueueIcon from "@mui/icons-material/AddToQueue";
import AlbumIcon from "@mui/icons-material/Album";
import PersonIcon from "@mui/icons-material/Person";
import DownloadIcon from "@mui/icons-material/Download";
import EditIcon from "@mui/icons-material/Edit";
import DeleteIcon from "@mui/icons-material/Delete";
import { tokens } from "../../theme";
import type { Track } from "./types";
import { formatDuration } from "./media";
import { useLibrary } from "./library";
import { usePlayer } from "./player";

/** A soft two-tone colour derived from a name, so covers without art differ. */
export function coverColors(name: string): [string, string] {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
  const hue = h % 360;
  return [`hsl(${hue} 38% 72%)`, `hsl(${(hue + 40) % 360} 42% 56%)`];
}

interface CoverProps {
  url?: string;
  name: string;
  size?: number | string | Record<string, number>;
  radius?: number | string;
  round?: boolean;
  icon?: ReactNode;
}

/** Cover shows the artwork, or a coloured tile with a note when there is none. */
export function Cover({ url, name, size = 48, radius = 8, round, icon }: CoverProps) {
  const [a, b] = coverColors(name || "?");
  return (
    <Box
      sx={{
        width: size,
        height: size,
        flexShrink: 0,
        borderRadius: round ? "50%" : `${radius}px`,
        overflow: "hidden",
        position: "relative",
        background: `linear-gradient(135deg, ${a}, ${b})`,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        color: "rgba(255,255,255,.9)",
      }}
    >
      {url ? (
        <img src={url} alt="" loading="lazy" style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }} />
      ) : (
        (icon ?? <MusicNoteRoundedIcon sx={{ fontSize: typeof size === "number" ? Math.max(18, size * 0.45) : 40 }} />)
      )}
    </Box>
  );
}

/** Shelf is a titled horizontal row of cards that scrolls sideways. */
export function Shelf({ title, subtitle, action, children, testId }: { title: string; subtitle?: string; action?: ReactNode; children: ReactNode; testId?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const scroll = (dir: number) => ref.current?.scrollBy({ left: dir * Math.max(240, (ref.current?.clientWidth ?? 600) * 0.8), behavior: "smooth" });
  return (
    <Box component="section" sx={{ mb: 4 }} data-testid={testId}>
      <Box sx={{ display: "flex", alignItems: "flex-end", gap: 1, mb: 1.5 }}>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          {subtitle && (
            <Typography level="body-xs" sx={{ textTransform: "uppercase", letterSpacing: ".06em", color: "text.tertiary", fontWeight: 600 }}>
              {subtitle}
            </Typography>
          )}
          <Typography level="h3" sx={{ fontSize: { xs: "xl", sm: "xl2" }, lineHeight: 1.15 }}>
            {title}
          </Typography>
        </Box>
        {action}
        <Box sx={{ display: { xs: "none", md: "flex" }, gap: 0.5 }}>
          <IconButton size="sm" variant="outlined" color="neutral" aria-label={`Scroll ${title} left`} onClick={() => scroll(-1)} sx={{ borderRadius: "50%" }}>
            <ChevronLeftRoundedIcon />
          </IconButton>
          <IconButton size="sm" variant="outlined" color="neutral" aria-label={`Scroll ${title} right`} onClick={() => scroll(1)} sx={{ borderRadius: "50%" }}>
            <ChevronRightRoundedIcon />
          </IconButton>
        </Box>
      </Box>
      <Box
        ref={ref}
        sx={{
          display: "flex",
          gap: 2,
          overflowX: "auto",
          pb: 1,
          scrollSnapType: "x proximity",
          scrollbarWidth: "thin",
          "& > *": { scrollSnapAlign: "start" },
        }}
      >
        {children}
      </Box>
    </Box>
  );
}

/** MediaCard is a square cover with a title, a line of detail and a play button on hover. */
export function MediaCard({
  art,
  title,
  sub,
  onOpen,
  onPlay,
  round,
  width = 168,
  playing,
  coverName,
  testId,
}: {
  art?: string;
  title: string;
  sub?: string;
  onOpen?: () => void;
  onPlay?: () => void;
  round?: boolean;
  width?: number;
  playing?: boolean;
  coverName?: string;
  testId?: string;
}) {
  return (
    <Box
      data-testid={testId}
      sx={{
        width,
        flexShrink: 0,
        cursor: "pointer",
        "&:hover .mc-play, &:focus-within .mc-play": { opacity: 1, transform: "none" },
        "&:hover .mc-cover": { boxShadow: "0 8px 22px rgba(14,36,32,.16)" },
      }}
    >
      <Box sx={{ position: "relative" }} onClick={onOpen ?? onPlay}>
        <Box className="mc-cover" sx={{ borderRadius: round ? "50%" : "12px", transition: "box-shadow .15s", lineHeight: 0 }}>
          <Cover url={art} name={coverName ?? title} size={width} radius={12} round={round} />
        </Box>
        {onPlay && (
          <IconButton
            className="mc-play"
            aria-label={`Play ${title}`}
            onClick={(e) => {
              e.stopPropagation();
              onPlay();
            }}
            sx={{
              position: "absolute",
              right: 8,
              bottom: 8,
              width: 42,
              height: 42,
              borderRadius: "50%",
              bgcolor: "primary.solidBg",
              color: "#fff",
              opacity: playing ? 1 : { xs: 1, md: 0 },
              transform: { md: "translateY(6px)" },
              transition: "all .15s",
              boxShadow: "0 4px 12px rgba(0,0,0,.25)",
              "&:hover": { bgcolor: "primary.solidHoverBg" },
            }}
          >
            {playing ? <PauseRoundedIcon /> : <PlayArrowRoundedIcon />}
          </IconButton>
        )}
      </Box>
      <Typography level="title-sm" noWrap sx={{ mt: 1, fontWeight: 600 }} onClick={onOpen ?? onPlay}>
        {title}
      </Typography>
      {sub && (
        <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
          {sub}
        </Typography>
      )}
    </Box>
  );
}

/** Chips renders a single-select row of pill chips. */
export function ChipRow<T extends string>({ value, options, onChange, label }: { value: T; options: { value: T; label: string }[]; onChange: (v: T) => void; label: string }) {
  return (
    <Box role="tablist" aria-label={label} sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
      {options.map((o) => (
        <Chip
          key={o.value}
          role="tab"
          aria-selected={value === o.value}
          size="lg"
          variant={value === o.value ? "solid" : "outlined"}
          color={value === o.value ? "primary" : "neutral"}
          onClick={() => onChange(o.value)}
          sx={{ cursor: "pointer", "--Chip-radius": "999px", fontWeight: 600 }}
        >
          {o.label}
        </Chip>
      ))}
    </Box>
  );
}

/** TrackMenu is the "..." menu of a song. */
export function TrackMenu({ track, onGoAlbum, onGoArtist, extra }: { track: Track; onGoAlbum?: () => void; onGoArtist?: () => void; extra?: ReactNode }) {
  const lib = useLibrary();
  const a = lib.actions;
  const mine = lib.userId === track.owner_id;
  return (
    <Dropdown>
      <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", color: "neutral", "aria-label": `More for ${track.title}` } }}>
        <MoreHorizRoundedIcon />
      </MenuButton>
      <Menu placement="bottom-end" size="sm" sx={{ minWidth: 200, zIndex: 1400 }}>
        <MenuItem onClick={() => a.playNext(track)}>
          <QueuePlayNextIcon fontSize="small" /> Play next
        </MenuItem>
        <MenuItem onClick={() => a.addToQueue(track)}>
          <AddToQueueIcon fontSize="small" /> Add to queue
        </MenuItem>
        <MenuItem onClick={() => a.addToPlaylist(track)}>
          <PlaylistAddIcon fontSize="small" /> Add to playlist
        </MenuItem>
        <MenuItem onClick={() => a.toggleLike(track)}>
          {track.liked ? <FavoriteIcon fontSize="small" color="error" /> : <FavoriteBorderIcon fontSize="small" />} {track.liked ? "Remove like" : "Like"}
        </MenuItem>
        {(onGoAlbum || onGoArtist) && <ListDivider />}
        {onGoAlbum && (
          <MenuItem onClick={onGoAlbum}>
            <AlbumIcon fontSize="small" /> Go to album
          </MenuItem>
        )}
        {onGoArtist && (
          <MenuItem onClick={onGoArtist}>
            <PersonIcon fontSize="small" /> Go to artist
          </MenuItem>
        )}
        <ListDivider />
        <MenuItem onClick={() => a.download(track)}>
          <DownloadIcon fontSize="small" /> Download
        </MenuItem>
        {mine && (
          <MenuItem onClick={() => a.edit(track)}>
            <EditIcon fontSize="small" /> Edit details
          </MenuItem>
        )}
        {mine && (
          <MenuItem color="danger" onClick={() => a.remove(track)}>
            <DeleteIcon fontSize="small" /> Delete
          </MenuItem>
        )}
        {extra}
      </Menu>
    </Dropdown>
  );
}

/** SongRow is one line of a song list. */
export function SongRow({
  track,
  index,
  onPlay,
  showAlbum = true,
  showArt = true,
  onGoAlbum,
  onGoArtist,
  extra,
  dragProps,
  rowExtra,
}: {
  track: Track;
  index?: number;
  onPlay: () => void;
  showAlbum?: boolean;
  showArt?: boolean;
  onGoAlbum?: () => void;
  onGoArtist?: () => void;
  extra?: ReactNode;
  dragProps?: Record<string, unknown>;
  rowExtra?: ReactNode;
}) {
  const p = usePlayer();
  const lib = useLibrary();
  const active = p.current?.id === track.id;
  const playingNow = active && p.playing;
  return (
    <Box
      {...dragProps}
      data-testid={`song-${track.id}`}
      data-active={active || undefined}
      onDoubleClick={onPlay}
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: `${index != null ? "24px " : ""}${showArt ? "44px " : ""}minmax(0,1fr) 28px 28px`,
          md: `${index != null ? "32px " : ""}${showArt ? "44px " : ""}minmax(0,2.2fr) ${showAlbum ? "minmax(0,1.4fr) " : ""}32px 56px 32px`,
        },
        alignItems: "center",
        gap: { xs: 1, md: 1.5 },
        px: 1,
        py: 0.75,
        borderRadius: "10px",
        bgcolor: active ? "primary.softBg" : "transparent",
        "&:hover": { bgcolor: active ? "primary.softBg" : "background.level1" },
        "&:hover .sr-num": { display: "none" },
        "&:hover .sr-play": { display: "flex" },
      }}
    >
      {index != null && (
        <Box sx={{ position: "relative", textAlign: "center", color: "text.tertiary", fontSize: "sm" }}>
          <span className="sr-num" style={{ display: active ? "none" : undefined }}>
            {index + 1}
          </span>
          <IconButton className="sr-play" size="sm" variant="plain" aria-label={playingNow ? "Pause" : `Play ${track.title}`} onClick={() => (active ? p.toggle() : onPlay())} sx={{ display: active ? "flex" : "none", minWidth: 0, p: 0 }}>
            {playingNow ? <PauseRoundedIcon sx={{ color: "primary.solidBg" }} /> : <PlayArrowRoundedIcon />}
          </IconButton>
        </Box>
      )}
      {showArt && (
        <Box sx={{ position: "relative", cursor: "pointer", lineHeight: 0 }} onClick={() => (active ? p.toggle() : onPlay())}>
          <Cover url={track.art_url} name={track.album || track.title} size={44} radius={8} />
        </Box>
      )}
      <Box sx={{ minWidth: 0 }}>
        <Typography level="title-sm" noWrap sx={{ fontWeight: 600, color: active ? "primary.solidBg" : "text.primary" }}>
          {track.title || "Untitled"}
        </Typography>
        <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
          {track.artist || "Unknown artist"}
          {track.has_lyrics ? " · lyrics" : ""}
        </Typography>
      </Box>
      {showAlbum && (
        <Typography level="body-sm" noWrap sx={{ display: { xs: "none", md: "block" }, color: "text.secondary" }}>
          {track.album}
        </Typography>
      )}
      <IconButton size="sm" variant="plain" color={track.liked ? "danger" : "neutral"} aria-label={track.liked ? "Remove like" : "Like"} aria-pressed={!!track.liked} onClick={() => lib.actions.toggleLike(track)} sx={{ display: { xs: "none", md: "inline-flex" } }}>
        {track.liked ? <FavoriteIcon fontSize="small" /> : <FavoriteBorderIcon fontSize="small" />}
      </IconButton>
      <Typography level="body-xs" sx={{ display: { xs: "none", md: "block" }, color: "text.tertiary", textAlign: "right", fontVariantNumeric: "tabular-nums" }}>
        {formatDuration(track.duration_seconds)}
      </Typography>
      <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.25 }}>
        {rowExtra}
        <TrackMenu track={track} onGoAlbum={onGoAlbum} onGoArtist={onGoArtist} extra={extra} />
      </Box>
    </Box>
  );
}

/** PageTitle is the large heading at the top of a page. */
export function PageTitle({ title, sub, action }: { title: string; sub?: string; action?: ReactNode }) {
  return (
    <Box sx={{ display: "flex", alignItems: "flex-end", gap: 2, mb: 2.5, flexWrap: "wrap" }}>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography level="h1" sx={{ fontFamily: tokens.font.display, fontSize: { xs: "xl3", sm: "xl4" }, letterSpacing: "-0.02em" }}>
          {title}
        </Typography>
        {sub && (
          <Typography level="body-sm" sx={{ color: "text.tertiary", mt: 0.5 }}>
            {sub}
          </Typography>
        )}
      </Box>
      {action}
    </Box>
  );
}

export function EmptyNote({ icon, title, text, action }: { icon?: ReactNode; title: string; text?: string; action?: ReactNode }) {
  return (
    <Box sx={{ textAlign: "center", py: 8, px: 2, border: "1px dashed", borderColor: "divider", borderRadius: "16px", bgcolor: "background.level1" }}>
      <Box sx={{ color: "text.tertiary", mb: 1, "& svg": { fontSize: 44 } }}>{icon ?? <MusicNoteRoundedIcon />}</Box>
      <Typography level="title-md">{title}</Typography>
      {text && (
        <Typography level="body-sm" sx={{ color: "text.tertiary", mt: 0.5, maxWidth: 420, mx: "auto" }}>
          {text}
        </Typography>
      )}
      {action && <Box sx={{ mt: 2 }}>{action}</Box>}
    </Box>
  );
}
