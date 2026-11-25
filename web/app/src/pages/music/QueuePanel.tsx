import { Box, Typography, IconButton, Button, Dropdown, Menu, MenuButton, MenuItem } from "@mui/joy";
import CloseRoundedIcon from "@mui/icons-material/CloseRounded";
import DragIndicatorRoundedIcon from "@mui/icons-material/DragIndicatorRounded";
import MoreVertRoundedIcon from "@mui/icons-material/MoreVertRounded";
import GraphicEqRoundedIcon from "@mui/icons-material/GraphicEqRounded";
import { usePlayer } from "./player";
import { Cover } from "./ui";
import { useDragReorder } from "./dnd";
import { formatDuration } from "./media";

/** QueuePanel lists the play queue: drag to reorder, play, move, remove. */
export function QueuePanel({ onClose }: { onClose: () => void }) {
  const p = usePlayer();
  const dnd = useDragReorder((from, to) => p.moveInQueue(from, to));
  const upcoming = p.queue.length - Math.max(p.index + 1, 0);
  return (
    <Box
      role="complementary"
      aria-label="Queue"
      data-testid="queue-panel"
      sx={{
        position: "fixed",
        zIndex: 1200,
        top: { xs: 0, md: "var(--ad-topbar-h, 56px)" },
        right: 0,
        bottom: { xs: 0, md: 80 },
        width: { xs: "100%", sm: 380 },
        bgcolor: "background.surface",
        borderLeft: { sm: "1px solid" },
        borderColor: "divider",
        boxShadow: "-12px 0 32px rgba(14,36,32,.10)",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 2, pb: 1 }}>
        <Typography level="title-lg" sx={{ flex: 1 }}>
          Queue
        </Typography>
        <Button size="sm" variant="plain" color="neutral" onClick={p.clearQueue} disabled={!p.queue.length}>
          Clear
        </Button>
        <IconButton size="sm" variant="plain" color="neutral" aria-label="Close queue" onClick={onClose}>
          <CloseRoundedIcon />
        </IconButton>
      </Box>
      <Typography level="body-xs" sx={{ px: 2, pb: 1, color: "text.tertiary" }}>
        {p.queue.length === 0 ? "Nothing queued." : `${p.queue.length} songs, ${Math.max(upcoming, 0)} up next. Drag the handle to reorder.`}
      </Typography>
      <Box sx={{ flex: 1, overflowY: "auto", px: 1, pb: 2 }}>
        {p.queue.map((t, i) => {
          const active = i === p.index;
          const rp = dnd.rowProps(i);
          return (
            <Box
              key={`${t.id}-${i}`}
              {...rp}
              data-testid={`queue-row-${i}`}
              data-active={active || undefined}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1,
                p: 0.75,
                borderRadius: "10px",
                bgcolor: active ? "primary.softBg" : "transparent",
                opacity: dnd.dragging === i ? 0.4 : 1,
                borderTop: dnd.over === i && dnd.dragging !== null && dnd.dragging !== i ? "2px solid" : "2px solid transparent",
                borderTopColor: dnd.over === i && dnd.dragging !== null && dnd.dragging !== i ? "primary.solidBg" : "transparent",
                "&:hover": { bgcolor: active ? "primary.softBg" : "background.level1" },
              }}
            >
              <DragIndicatorRoundedIcon fontSize="small" sx={{ color: "text.tertiary", cursor: "grab", flexShrink: 0 }} aria-hidden />
              <Box sx={{ cursor: "pointer", lineHeight: 0, position: "relative" }} onClick={() => p.playAt(i)}>
                <Cover url={t.art_url} name={t.album || t.title} size={40} radius={8} />
                {active && p.playing && (
                  <Box sx={{ position: "absolute", inset: 0, bgcolor: "rgba(0,0,0,.35)", borderRadius: "8px", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff" }}>
                    <GraphicEqRoundedIcon fontSize="small" />
                  </Box>
                )}
              </Box>
              <Box sx={{ minWidth: 0, flex: 1, cursor: "pointer" }} onClick={() => p.playAt(i)}>
                <Typography level="title-sm" noWrap sx={{ fontWeight: 600, color: active ? "primary.solidBg" : "text.primary" }}>
                  {t.title}
                </Typography>
                <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
                  {t.artist || "Unknown artist"} · {formatDuration(t.duration_seconds)}
                </Typography>
              </Box>
              <Dropdown>
                <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", color: "neutral", "aria-label": `Queue actions for ${t.title}` } }}>
                  <MoreVertRoundedIcon fontSize="small" />
                </MenuButton>
                <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1400 }}>
                  <MenuItem onClick={() => p.playAt(i)}>Play now</MenuItem>
                  <MenuItem disabled={i === 0} onClick={() => p.moveInQueue(i, i - 1)}>
                    Move up
                  </MenuItem>
                  <MenuItem disabled={i === p.queue.length - 1} onClick={() => p.moveInQueue(i, i + 1)}>
                    Move down
                  </MenuItem>
                  <MenuItem color="danger" onClick={() => p.removeAtIndex(i)}>
                    Remove from queue
                  </MenuItem>
                </Menu>
              </Dropdown>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
}
