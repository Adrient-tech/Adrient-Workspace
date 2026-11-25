import { Box, Typography, IconButton, Slider, Dropdown, Menu, MenuButton, MenuItem, Tooltip } from "@mui/joy";
import PlayArrowRoundedIcon from "@mui/icons-material/PlayArrowRounded";
import PauseRoundedIcon from "@mui/icons-material/PauseRounded";
import SkipNextRoundedIcon from "@mui/icons-material/SkipNextRounded";
import SkipPreviousRoundedIcon from "@mui/icons-material/SkipPreviousRounded";
import ShuffleRoundedIcon from "@mui/icons-material/ShuffleRounded";
import RepeatRoundedIcon from "@mui/icons-material/RepeatRounded";
import RepeatOneRoundedIcon from "@mui/icons-material/RepeatOneRounded";
import VolumeUpRoundedIcon from "@mui/icons-material/VolumeUpRounded";
import VolumeOffRoundedIcon from "@mui/icons-material/VolumeOffRounded";
import VolumeDownRoundedIcon from "@mui/icons-material/VolumeDownRounded";
import QueueMusicRoundedIcon from "@mui/icons-material/QueueMusicRounded";
import FavoriteIcon from "@mui/icons-material/Favorite";
import FavoriteBorderIcon from "@mui/icons-material/FavoriteBorder";
import PlaylistAddIcon from "@mui/icons-material/PlaylistAdd";
import SpeedRoundedIcon from "@mui/icons-material/SpeedRounded";
import KeyboardArrowUpRoundedIcon from "@mui/icons-material/KeyboardArrowUpRounded";
import RadioIcon from "@mui/icons-material/Radio";
import CloseIcon from "@mui/icons-material/Close";
import { usePlayer } from "./player";
import { useLibrary } from "./library";
import { Cover } from "./ui";
import { formatClock } from "./media";

export const SPEEDS = [0.75, 1, 1.25, 1.5, 2];
export const PLAYER_H = 80;

interface Props {
  queueOpen: boolean;
  onToggleQueue: () => void;
  onOpenNow: () => void;
  nowOpen: boolean;
}

/** PlayerBar is the persistent bar at the bottom of the Music app. */
export function PlayerBar({ queueOpen, onToggleQueue, onOpenNow, nowOpen }: Props) {
  const p = usePlayer();
  const lib = useLibrary();
  const t = p.current;
  const radio = p.radioStation;
  if (!t && !radio) return null;

  const VolIcon = p.muted || p.volume === 0 ? VolumeOffRoundedIcon : p.volume < 0.5 ? VolumeDownRoundedIcon : VolumeUpRoundedIcon;
  const RepeatIcon = p.repeat === "one" ? RepeatOneRoundedIcon : RepeatRoundedIcon;
  const dur = p.duration || t?.duration_seconds || 0;

  return (
    <Box
      role="region"
      aria-label="Music player"
      data-testid="player-bar"
      sx={{
        position: "fixed",
        bottom: { xs: 56, md: 0 },
        left: { xs: 0, md: "var(--ad-rail-w, 0px)" },
        right: 0,
        zIndex: 1100,
        height: PLAYER_H,
        bgcolor: "rgba(255,255,255,.96)",
        backdropFilter: "blur(10px)",
        borderTop: "1px solid",
        borderColor: "divider",
        display: "grid",
        gridTemplateColumns: { xs: "minmax(0,1fr) auto", md: "minmax(220px,1fr) minmax(320px,2fr) minmax(220px,1fr)" },
        alignItems: "center",
        gap: { xs: 1, md: 2 },
        px: { xs: 1.5, md: 2.5 },
        // A thin progress line on phones, where the seek bar is hidden.
        "&::before": {
          content: '""',
          display: { xs: "block", md: "none" },
          position: "absolute",
          top: -1,
          left: 0,
          height: 2,
          width: `${dur ? Math.min(100, (p.position / dur) * 100) : 0}%`,
          bgcolor: "primary.solidBg",
        },
      }}
    >
      {/* Track */}
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, minWidth: 0 }}>
        {radio ? (
          <Cover name={radio.name} size={52} radius={10} icon={<RadioIcon />} />
        ) : (
          <Box component="button" onClick={onOpenNow} aria-label="Open now playing" sx={{ all: "unset", cursor: "pointer", lineHeight: 0 }}>
            <Cover url={t?.art_url} name={t?.album || t?.title || "?"} size={52} radius={10} />
          </Box>
        )}
        <Box sx={{ minWidth: 0 }}>
          <Typography level="title-sm" noWrap sx={{ fontWeight: 700 }} data-testid="player-title">
            {radio ? radio.name : t?.title || "Untitled"}
          </Typography>
          <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
            {radio ? "Live radio" : t?.artist || "Unknown artist"}
          </Typography>
        </Box>
        {t && !radio && (
          <Box sx={{ display: { xs: "none", md: "flex" }, ml: 0.5 }}>
            <Tooltip title={t.liked ? "Remove like" : "Like"} size="sm">
              <IconButton size="sm" variant="plain" color={t.liked ? "danger" : "neutral"} aria-pressed={!!t.liked} aria-label={t.liked ? "Remove like" : "Like"} onClick={() => lib.actions.toggleLike(t)}>
                {t.liked ? <FavoriteIcon fontSize="small" /> : <FavoriteBorderIcon fontSize="small" />}
              </IconButton>
            </Tooltip>
            <Tooltip title="Add to playlist" size="sm">
              <IconButton size="sm" variant="plain" color="neutral" aria-label="Add to playlist" onClick={() => lib.actions.addToPlaylist(t)}>
                <PlaylistAddIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Box>
        )}
      </Box>

      {/* Transport + seek */}
      <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 0.25, minWidth: 0 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 0.5, md: 1 } }}>
          {!radio && (
            <IconButton size="sm" variant="plain" color={p.shuffle ? "primary" : "neutral"} aria-pressed={p.shuffle} aria-label="Shuffle" onClick={p.toggleShuffle} sx={{ display: { xs: "none", md: "inline-flex" } }}>
              <ShuffleRoundedIcon />
            </IconButton>
          )}
          {!radio && (
            <IconButton size="md" variant="plain" color="neutral" aria-label="Previous" onClick={p.prev} sx={{ display: { xs: "none", md: "inline-flex" } }}>
              <SkipPreviousRoundedIcon />
            </IconButton>
          )}
          <IconButton
            aria-label={p.playing ? "Pause" : "Play"}
            data-testid="player-toggle"
            onClick={p.toggle}
            sx={{ width: 44, height: 44, borderRadius: "50%", bgcolor: "primary.solidBg", color: "#fff", "&:hover": { bgcolor: "primary.solidHoverBg" } }}
          >
            {p.playing ? <PauseRoundedIcon /> : <PlayArrowRoundedIcon />}
          </IconButton>
          {!radio ? (
            <IconButton size="md" variant="plain" color="neutral" aria-label="Next" onClick={p.next}>
              <SkipNextRoundedIcon />
            </IconButton>
          ) : (
            <IconButton size="sm" variant="plain" color="neutral" aria-label="Stop radio" onClick={p.stopRadio}>
              <CloseIcon />
            </IconButton>
          )}
          {!radio && (
            <IconButton size="sm" variant="plain" color={p.repeat !== "off" ? "primary" : "neutral"} aria-label={`Repeat ${p.repeat}`} onClick={p.cycleRepeat} sx={{ display: { xs: "none", md: "inline-flex" } }}>
              <RepeatIcon />
            </IconButton>
          )}
        </Box>
        {!radio && (
          <Box sx={{ display: { xs: "none", md: "flex" }, alignItems: "center", gap: 1, width: "100%", maxWidth: 560 }}>
            <Typography level="body-xs" sx={{ width: 38, textAlign: "right", fontVariantNumeric: "tabular-nums", color: "text.tertiary" }}>
              {formatClock(p.position)}
            </Typography>
            <Slider
              size="sm"
              aria-label="Seek"
              data-testid="player-seek"
              value={Math.min(p.position, dur || 0)}
              min={0}
              max={dur || 1}
              step={0.5}
              onChange={(_, v) => p.seek(v as number)}
              sx={{ flex: 1, "--Slider-trackSize": "4px", "--Slider-thumbSize": "12px", py: 0.5 }}
            />
            <Typography level="body-xs" sx={{ width: 38, fontVariantNumeric: "tabular-nums", color: "text.tertiary" }}>
              {formatClock(dur)}
            </Typography>
          </Box>
        )}
      </Box>

      {/* Extras */}
      <Box sx={{ display: { xs: "none", md: "flex" }, alignItems: "center", justifyContent: "flex-end", gap: 0.5 }}>
        {!radio && (
          <Dropdown>
            <Tooltip title="Playback speed" size="sm">
              <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: p.speed !== 1 ? "soft" : "plain", color: p.speed !== 1 ? "primary" : "neutral", "aria-label": `Speed ${p.speed}x` } }}>
                {p.speed === 1 ? <SpeedRoundedIcon /> : <Typography level="body-xs" sx={{ fontWeight: 700 }}>{p.speed}x</Typography>}
              </MenuButton>
            </Tooltip>
            <Menu size="sm" placement="top-end" sx={{ zIndex: 1400 }}>
              {SPEEDS.map((s) => (
                <MenuItem key={s} selected={p.speed === s} onClick={() => p.setSpeed(s)}>
                  {s === 1 ? "Normal" : `${s}x`}
                </MenuItem>
              ))}
            </Menu>
          </Dropdown>
        )}
        <IconButton size="sm" variant="plain" color="neutral" aria-label={p.muted ? "Unmute" : "Mute"} onClick={p.toggleMute}>
          <VolIcon />
        </IconButton>
        <Slider
          size="sm"
          aria-label="Volume"
          value={p.muted ? 0 : p.volume}
          min={0}
          max={1}
          step={0.02}
          onChange={(_, v) => p.setVolume(v as number)}
          sx={{ width: 84, "--Slider-trackSize": "4px", "--Slider-thumbSize": "12px" }}
        />
        {!radio && (
          <Tooltip title="Queue (Q)" size="sm">
            <IconButton size="sm" variant={queueOpen ? "soft" : "plain"} color={queueOpen ? "primary" : "neutral"} aria-pressed={queueOpen} aria-label="Queue" data-testid="toggle-queue" onClick={onToggleQueue}>
              <QueueMusicRoundedIcon />
            </IconButton>
          </Tooltip>
        )}
        {!radio && (
          <Tooltip title="Now playing (N)" size="sm">
            <IconButton size="sm" variant={nowOpen ? "soft" : "plain"} color={nowOpen ? "primary" : "neutral"} aria-label="Now playing" onClick={onOpenNow}>
              <KeyboardArrowUpRoundedIcon sx={{ transform: nowOpen ? "rotate(180deg)" : "none" }} />
            </IconButton>
          </Tooltip>
        )}
      </Box>
    </Box>
  );
}
