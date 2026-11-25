// Slide sorter view: every slide as a thumbnail in a grid; drag to reorder,
// double-click to open in the editor.
import { useState } from "react";
import { Box, Typography } from "@mui/joy";
import { SlideView } from "./SlideView";
import { withFooters } from "./layouts";
import { sectionRanges } from "./sections";
import type { DeckDoc } from "./model";

interface Props {
  doc: DeckDoc;
  cur: number;
  onSelect: (i: number) => void;
  onOpen: (i: number) => void;
  onMove: (from: number, to: number) => void;
}

export function SlideSorter({ doc, cur, onSelect, onOpen, onMove }: Props) {
  const [drag, setDrag] = useState<number | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const groups = sectionRanges(doc);
  return (
    <Box data-testid="slide-sorter" sx={{ flex: 1, minWidth: 0, overflow: "auto", p: 2, bgcolor: "background.level1" }}>
      {groups.map((g) => (
        <Box key={g.section?.id ?? "start"} sx={{ mb: 2 }}>
          {g.section && (
            <Typography level="title-sm" sx={{ mb: 1, borderBottom: "2px solid", borderColor: "primary.500", pb: 0.25 }}>
              {g.section.name}
            </Typography>
          )}
          <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(200px, 1fr))", gap: 2 }}>
            {doc.slides.slice(g.start, g.end).map((s, k) => {
              const i = g.start + k;
              return (
                <Box
                  key={s.id}
                  data-testid="sorter-thumb"
                  role="button"
                  tabIndex={0}
                  aria-label={`Slide ${i + 1}${s.hidden ? " (skipped)" : ""}`}
                  aria-pressed={i === cur}
                  draggable
                  onDragStart={(e) => {
                    setDrag(i);
                    e.dataTransfer.effectAllowed = "move";
                  }}
                  onDragOver={(e) => {
                    e.preventDefault();
                    setOver(i);
                  }}
                  onDragEnd={() => {
                    setDrag(null);
                    setOver(null);
                  }}
                  onDrop={(e) => {
                    e.preventDefault();
                    if (drag !== null && drag !== i) onMove(drag, i);
                    setDrag(null);
                    setOver(null);
                  }}
                  onClick={() => onSelect(i)}
                  onDoubleClick={() => onOpen(i)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") onOpen(i);
                    if (e.key === " ") {
                      e.preventDefault();
                      onSelect(i);
                    }
                  }}
                  sx={{
                    cursor: "grab",
                    border: "2px solid",
                    borderColor: i === cur ? "primary.500" : over === i && drag !== null ? "warning.400" : "transparent",
                    borderRadius: 6,
                    p: 0.5,
                    bgcolor: "background.body",
                    opacity: drag === i ? 0.4 : 1,
                  }}
                >
                  <Box sx={{ lineHeight: 0, opacity: s.hidden ? 0.45 : 1 }}>
                    <SlideView slide={withFooters(doc, i)} width={190} />
                  </Box>
                  <Typography level="body-xs" sx={{ mt: 0.5 }}>
                    {i + 1}
                    {s.hidden ? " · skipped" : ""}
                    {s.advanceAfter ? ` · ${Math.round(s.advanceAfter / 1000)}s` : ""}
                    {s.transition && s.transition !== "none" ? ` · ${s.transition}` : ""}
                  </Typography>
                </Box>
              );
            })}
          </Box>
        </Box>
      ))}
    </Box>
  );
}
