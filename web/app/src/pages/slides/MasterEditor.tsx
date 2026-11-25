// Slide master view: edit the deck's layouts on the same canvas as slides.
// Edits are kept in local state and written to the deck on "Apply".
import { useMemo, useState } from "react";
import { Box, Button, Dropdown, IconButton, Input, Menu, MenuButton, MenuItem, Modal, ModalDialog, Typography } from "@mui/joy";
import { SlideCanvas } from "./SlideCanvas";
import { SlideView } from "./SlideView";
import { layoutsOf } from "./layouts";
import { CANVAS_W, newElement, type DeckDoc, type ElementType, type PlaceholderType, type SlideElement } from "./model";
import {
  PLACEHOLDER_KINDS, addPlaceholder, deleteLayout, duplicateLayout, layoutToSlide, layoutUsage, moveLayout, newLayout, slideToLayout,
} from "./masterOps";
import { isShape } from "./model";

interface Props {
  doc: DeckDoc;
  initialLayout?: string;
  onApply: (next: DeckDoc) => void;
  onClose: () => void;
  onEditTheme: () => void;
}

export function MasterEditor({ doc, initialLayout, onApply, onClose, onEditTheme }: Props) {
  const [work, setWork] = useState<DeckDoc>(() => ({ ...doc, layouts: layoutsOf(doc) }));
  const [curId, setCurId] = useState(() => (initialLayout && layoutsOf(doc).some((l) => l.id === initialLayout) ? initialLayout : layoutsOf(doc)[0].id));
  const [sel, setSel] = useState<string[]>([]);
  const layouts = layoutsOf(work);
  const layout = layouts.find((l) => l.id === curId) ?? layouts[0];
  const slide = useMemo(() => layoutToSlide(layout), [layout]);
  const usage = layoutUsage(work);
  const selEl = layout.elements.find((e) => e.id === sel[sel.length - 1]);

  const setLayout = (next: typeof layout) => setWork((w) => ({ ...w, layouts: layoutsOf(w).map((l) => (l.id === next.id ? next : l)) }));
  const setEls = (elements: SlideElement[]) => setLayout({ ...layout, elements });
  const upsert = (els: SlideElement[]) => {
    const byId = new Map(els.map((e) => [e.id, e]));
    const merged = layout.elements.map((e) => byId.get(e.id) ?? e);
    for (const e of els) if (!layout.elements.some((x) => x.id === e.id)) merged.push(e);
    setEls(merged);
  };
  const patchSel = (p: Partial<SlideElement>) => selEl && upsert([{ ...selEl, ...p }]);
  const addEl = (t: ElementType) => {
    const el = newElement(t);
    upsert([el]);
    setSel([el.id]);
  };
  const addPh = (t: PlaceholderType) => {
    const next = addPlaceholder(layout, t, work);
    setLayout(next);
    setSel([next.elements[next.elements.length - 1].id]);
  };

  return (
    <Modal open onClose={onClose}>
      <ModalDialog layout="fullscreen" aria-labelledby="master-title" sx={{ p: 0, display: "flex", flexDirection: "column" }} data-testid="master-editor">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, px: 2, py: 1, borderBottom: "1px solid", borderColor: "divider", flexWrap: "wrap" }}>
          <Typography id="master-title" level="title-md">
            Slide master
          </Typography>
          <Dropdown>
            <MenuButton size="sm" variant="outlined" aria-label="Insert placeholder">
              Insert placeholder ▾
            </MenuButton>
            <Menu size="sm" sx={{ zIndex: 1500 }}>
              {PLACEHOLDER_KINDS.map((k) => (
                <MenuItem key={k.type} onClick={() => addPh(k.type)}>
                  {k.label}
                </MenuItem>
              ))}
            </Menu>
          </Dropdown>
          <Button size="sm" variant="outlined" onClick={() => addEl("text")}>
            Text box
          </Button>
          <Button size="sm" variant="outlined" onClick={() => addEl("rect")}>
            Rectangle
          </Button>
          <Button size="sm" variant="outlined" onClick={() => addEl("ellipse")}>
            Ellipse
          </Button>
          <Button size="sm" variant="outlined" color="danger" disabled={!sel.length} onClick={() => { setEls(layout.elements.filter((e) => !sel.includes(e.id))); setSel([]); }}>
            Delete
          </Button>
          <Box component="label" sx={{ display: "flex", alignItems: "center", gap: 0.5, fontSize: 13 }}>
            Background
            <input
              type="color"
              aria-label="Layout background colour"
              value={/^#[0-9a-f]{6}/i.test(slide.background) ? slide.background.slice(0, 7) : "#ffffff"}
              onChange={(e) => setLayout(slideToLayout(layout, { ...slide, background: e.target.value, bgRef: undefined, bgFill: undefined }))}
            />
          </Box>
          {selEl && selEl.type === "text" && (
            <>
              <Box component="label" sx={{ display: "flex", alignItems: "center", gap: 0.5, fontSize: 13 }}>
                Size
                <Input size="sm" type="number" value={selEl.fontSize ?? 18} onChange={(e) => patchSel({ fontSize: Math.max(6, Math.min(200, Number(e.target.value) || 18)) })} sx={{ width: 72 }} slotProps={{ input: { "aria-label": "Font size" } }} />
              </Box>
              <Box component="label" sx={{ display: "flex", alignItems: "center", gap: 0.5, fontSize: 13 }}>
                Text colour
                <input type="color" aria-label="Text colour" value={/^#[0-9a-f]{6}/i.test(selEl.color ?? "") ? selEl.color!.slice(0, 7) : "#000000"} onChange={(e) => patchSel({ color: e.target.value })} />
              </Box>
              <Button size="sm" variant={selEl.bold ? "soft" : "plain"} onClick={() => patchSel({ bold: !selEl.bold })} aria-label="Bold">
                B
              </Button>
            </>
          )}
          {selEl && isShape(selEl.type) && (
            <Box component="label" sx={{ display: "flex", alignItems: "center", gap: 0.5, fontSize: 13 }}>
              Fill
              <input type="color" aria-label="Shape fill" value={/^#[0-9a-f]{6}/i.test(selEl.fill ?? "") ? selEl.fill!.slice(0, 7) : "#126C5A"} onChange={(e) => patchSel({ fill: e.target.value })} />
            </Box>
          )}
          <Button size="sm" variant="plain" onClick={() => { onApply(work); onEditTheme(); }}>
            Colours and fonts…
          </Button>
          <Box sx={{ flex: 1 }} />
          <Button size="sm" variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button size="sm" data-testid="master-apply" onClick={() => onApply(work)}>
            Apply and close
          </Button>
        </Box>
        <Box sx={{ flex: 1, minHeight: 0, display: "flex" }}>
          <Box sx={{ width: 220, borderRight: "1px solid", borderColor: "divider", overflowY: "auto", p: 1, bgcolor: "background.level1" }} data-testid="master-layouts">
            {layouts.map((l) => (
              <Box
                key={l.id}
                role="button"
                tabIndex={0}
                aria-pressed={l.id === layout.id}
                data-testid="master-layout"
                onClick={() => { setCurId(l.id); setSel([]); }}
                onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { setCurId(l.id); setSel([]); } }}
                sx={{ mb: 1, cursor: "pointer", border: "2px solid", borderColor: l.id === layout.id ? "primary.500" : "transparent", borderRadius: 6, p: 0.5 }}
              >
                <Box sx={{ lineHeight: 0 }}>
                  <SlideView slide={layoutToSlide(l)} width={196} />
                </Box>
                <Typography level="body-xs" noWrap>
                  {l.name} {usage[l.id] ? `· used by ${usage[l.id]}` : ""}
                </Typography>
              </Box>
            ))}
            <Button size="sm" fullWidth variant="soft" onClick={() => { const nl = newLayout(work); setWork((w) => ({ ...w, layouts: [...layoutsOf(w), nl] })); setCurId(nl.id); }}>
              New layout
            </Button>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", alignItems: "center", p: 2, overflow: "auto", bgcolor: "background.level2" }}>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1, alignSelf: "stretch", flexWrap: "wrap" }}>
              <Input size="sm" value={layout.name} onChange={(e) => setWork((w) => ({ ...w, layouts: layoutsOf(w).map((l) => (l.id === layout.id ? { ...l, name: e.target.value } : l)) }))} slotProps={{ input: { "aria-label": "Layout name" } }} sx={{ width: 240 }} />
              <Button size="sm" variant="plain" onClick={() => setWork((w) => duplicateLayout(w, layout.id))}>Duplicate</Button>
              <IconButton size="sm" variant="plain" aria-label="Move layout up" onClick={() => setWork((w) => moveLayout(w, layout.id, -1))}>↑</IconButton>
              <IconButton size="sm" variant="plain" aria-label="Move layout down" onClick={() => setWork((w) => moveLayout(w, layout.id, 1))}>↓</IconButton>
              <Button size="sm" variant="plain" color="danger" disabled={layouts.length <= 1} onClick={() => { const next = deleteLayout(work, layout.id); setWork(next); setCurId(layoutsOf(next)[0].id); setSel([]); }}>
                Delete layout{usage[layout.id] ? ` (unlinks ${usage[layout.id]} slides)` : ""}
              </Button>
            </Box>
            <SlideCanvas
              slide={slide}
              width={Math.min(760, Math.max(360, typeof window !== "undefined" ? window.innerWidth - 320 : 760), CANVAS_W)}
              selectedIds={sel}
              onSelect={setSel}
              onChange={(el) => upsert([el])}
              onChangeMany={(els) => upsert(els)}
              snap={{ guides: true, grid: 0 }}
              a11yLabel={`Layout ${layout.name}, editing canvas`}
            />
          </Box>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
