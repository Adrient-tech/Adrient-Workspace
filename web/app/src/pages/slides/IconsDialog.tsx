import { createElement, useMemo, useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { Box, Button, Input, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import { ICON_LIBRARY, iconSvgDocument, searchIcons, type IconEntry } from "./iconLibrary";
import { newElement, type SlideElement } from "./model";

/** Render an icon to a PNG data URL (256 px). */
export async function rasterizeIcon(entry: IconEntry, color: string, px = 256): Promise<string> {
  const markup = renderToStaticMarkup(createElement(entry.Icon, {}));
  const svg = iconSvgDocument(markup, color, px);
  const img = new Image();
  const url = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
  await new Promise<void>((res, rej) => {
    img.onload = () => res();
    img.onerror = () => rej(new Error("Could not draw the icon"));
    img.src = url;
  });
  const c = document.createElement("canvas");
  c.width = px;
  c.height = px;
  c.getContext("2d")!.drawImage(img, 0, 0, px, px);
  return c.toDataURL("image/png");
}

export function IconsDialog({ onClose, onInsert }: { onClose: () => void; onInsert: (el: SlideElement) => void }) {
  const [q, setQ] = useState("");
  const [color, setColor] = useState("#126C5A");
  const [err, setErr] = useState("");
  const list = useMemo(() => searchIcons(q), [q]);
  async function pick(e: IconEntry) {
    try {
      const src = await rasterizeIcon(e, color);
      const el = newElement("image");
      onInsert({ ...el, x: 432, y: 222, w: 96, h: 96, src, name: `Icon: ${e.label}`, alt: e.label });
    } catch (x) {
      setErr((x as Error).message);
    }
  }
  return (
    <Modal open onClose={onClose}>
      <ModalDialog aria-labelledby="icons-title" sx={{ width: 640, maxWidth: "95vw", maxHeight: "85vh" }}>
        <ModalClose />
        <Typography id="icons-title" level="title-lg">
          Insert icon
        </Typography>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Input autoFocus placeholder="Search icons" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1 }} slotProps={{ input: { "aria-label": "Search icons" } }} />
          <input type="color" aria-label="Icon colour" value={color} onChange={(e) => setColor(e.target.value)} />
        </Box>
        {err && <Typography color="danger" level="body-sm">{err}</Typography>}
        <Box data-testid="icon-grid" sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(64px, 1fr))", gap: 0.5, overflowY: "auto", minHeight: 160 }}>
          {list.map((e) => (
            <Button key={e.name} variant="plain" color="neutral" title={e.label} aria-label={e.label} data-testid="icon-pick" onClick={() => void pick(e)} sx={{ flexDirection: "column", height: 64, fontSize: 10, gap: 0.25 }}>
              <e.Icon htmlColor={color} />
              <span style={{ maxWidth: 60, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{e.label}</span>
            </Button>
          ))}
          {!list.length && <Typography level="body-sm">No icons match “{q}”.</Typography>}
        </Box>
        <Typography level="body-xs">{ICON_LIBRARY.length} icons · inserted as pictures you can resize and recolour by re-inserting.</Typography>
      </ModalDialog>
    </Modal>
  );
}
