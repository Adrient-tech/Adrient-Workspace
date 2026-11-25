import { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, IconButton, Modal, ModalClose, ModalDialog, Tooltip, Typography } from "@mui/joy";
import ImageOutlinedIcon from "@mui/icons-material/ImageOutlined";
import BrushOutlinedIcon from "@mui/icons-material/BrushOutlined";
import CloseIcon from "@mui/icons-material/Close";
import UndoIcon from "@mui/icons-material/Undo";
import { attachmentUrl, checkImageFile, deleteAttachment, uploadAttachment, type AttachmentRef } from "./extrasApi";

const COLORS = ["#202124", "#d93025", "#188038", "#1a73e8", "#f9ab00", "#9334e6"];
const WIDTHS = [2, 5, 10];

interface Stroke {
  color: string;
  width: number;
  pts: [number, number][];
}

/** Freehand drawing canvas. Saves a PNG with a white background. */
export function DrawingDialog({ onSave, onClose }: { onSave: (png: Blob) => Promise<void>; onClose: () => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [strokes, setStrokes] = useState<Stroke[]>([]);
  const cur = useRef<Stroke | null>(null);
  const [color, setColor] = useState(COLORS[0]);
  const [width, setWidth] = useState(WIDTHS[1]);
  const [eraser, setEraser] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  function paint(list: Stroke[]) {
    const c = ref.current;
    if (!c) return;
    const g = c.getContext("2d")!;
    g.fillStyle = "#ffffff";
    g.fillRect(0, 0, c.width, c.height);
    g.lineCap = "round";
    g.lineJoin = "round";
    for (const s of list) {
      g.strokeStyle = s.color;
      g.fillStyle = s.color;
      g.lineWidth = s.width;
      if (s.pts.length === 1) {
        g.beginPath();
        g.arc(s.pts[0][0], s.pts[0][1], s.width / 2, 0, Math.PI * 2);
        g.fill();
        continue;
      }
      g.beginPath();
      s.pts.forEach(([x, y], i) => (i === 0 ? g.moveTo(x, y) : g.lineTo(x, y)));
      g.stroke();
    }
  }
  useEffect(() => paint(strokes), [strokes]);

  function pos(e: React.PointerEvent<HTMLCanvasElement>): [number, number] {
    const c = ref.current!;
    const r = c.getBoundingClientRect();
    return [((e.clientX - r.left) * c.width) / r.width, ((e.clientY - r.top) * c.height) / r.height];
  }
  function down(e: React.PointerEvent<HTMLCanvasElement>) {
    e.currentTarget.setPointerCapture(e.pointerId);
    cur.current = { color: eraser ? "#ffffff" : color, width: eraser ? width * 3 : width, pts: [pos(e)] };
    paint([...strokes, cur.current]);
  }
  function move(e: React.PointerEvent<HTMLCanvasElement>) {
    if (!cur.current) return;
    cur.current.pts.push(pos(e));
    paint([...strokes, cur.current]);
  }
  function up() {
    if (!cur.current) return;
    const s = cur.current;
    cur.current = null;
    setStrokes((l) => [...l, s]);
  }

  async function save() {
    const c = ref.current;
    if (!c || strokes.length === 0) return;
    setBusy(true);
    setErr("");
    try {
      const blob = await new Promise<Blob>((res, rej) => c.toBlob((b) => (b ? res(b) : rej(new Error("Could not make the image"))), "image/png"));
      await onSave(blob);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(860px, 98vw)", maxHeight: "98dvh", overflow: "auto" }} data-testid="drawing-dialog">
        <ModalClose />
        <Typography level="title-md">Drawing</Typography>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          {COLORS.map((c) => (
            <IconButton key={c} size="sm" aria-label={`Pen colour ${c}`} onClick={() => { setColor(c); setEraser(false); }}
              sx={{ bgcolor: c, width: 28, height: 28, minHeight: 0, borderRadius: "50%", outline: !eraser && color === c ? "3px solid" : "none", outlineColor: "primary.500", "&:hover": { bgcolor: c } }} />
          ))}
          {WIDTHS.map((w) => (
            <Button key={w} size="sm" variant={width === w ? "solid" : "outlined"} color="neutral" onClick={() => setWidth(w)} aria-label={`Pen size ${w}`}>
              {w === 2 ? "Thin" : w === 5 ? "Medium" : "Thick"}
            </Button>
          ))}
          <Button size="sm" variant={eraser ? "solid" : "outlined"} color="neutral" onClick={() => setEraser((v) => !v)}>Eraser</Button>
          <Button size="sm" variant="outlined" color="neutral" startDecorator={<UndoIcon />} disabled={!strokes.length} onClick={() => setStrokes((l) => l.slice(0, -1))} data-testid="draw-undo">Undo</Button>
          <Button size="sm" variant="outlined" color="neutral" disabled={!strokes.length} onClick={() => setStrokes([])}>Clear</Button>
        </Box>
        <canvas
          ref={ref}
          width={900}
          height={560}
          data-testid="draw-canvas"
          onPointerDown={down}
          onPointerMove={move}
          onPointerUp={up}
          onPointerCancel={up}
          style={{ width: "100%", aspectRatio: "900 / 560", border: "1px solid #ccc", borderRadius: 8, touchAction: "none", cursor: "crosshair", background: "#fff" }}
        />
        {err && <Alert color="danger">{err}</Alert>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button loading={busy} disabled={!strokes.length} onClick={save} data-testid="draw-save">Add to note</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

/** Read-only thumbnails shown on a note card. */
export function CardThumbs({ items }: { items: AttachmentRef[] }) {
  if (!items.length) return null;
  return (
    <Box sx={{ display: "grid", gridTemplateColumns: items.length === 1 ? "1fr" : "1fr 1fr", gap: 0.5, mb: 1, mx: -0.5 }} data-testid="card-thumbs">
      {items.slice(0, 4).map((a) => (
        <img key={a.id} src={attachmentUrl(a.id)} alt={a.kind === "drawing" ? "Drawing" : "Image"} loading="lazy" style={{ width: "100%", maxHeight: 220, objectFit: "cover", borderRadius: 8 }} />
      ))}
    </Box>
  );
}

/** Images and drawings of a note, with add and remove. Used in the editor. */
export function NoteMedia({ noteId, items, canEdit, onChange }: {
  noteId: string;
  items: AttachmentRef[];
  canEdit: boolean;
  onChange: (next: AttachmentRef[]) => void;
}) {
  const file = useRef<HTMLInputElement>(null);
  const [drawing, setDrawing] = useState(false);
  const [err, setErr] = useState("");

  async function addFile(f: File) {
    const bad = checkImageFile(f);
    if (bad) return setErr(bad);
    setErr("");
    try {
      const ref = await uploadAttachment(noteId, "image", f);
      onChange([...items, ref]);
    } catch (e) {
      setErr((e as Error).message);
    }
  }
  async function remove(a: AttachmentRef) {
    try {
      await deleteAttachment(a.id);
      onChange(items.filter((x) => x.id !== a.id));
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Box data-testid="note-media">
      {items.length > 0 && (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 1 }}>
          {items.map((a) => (
            <Box key={a.id} sx={{ position: "relative" }}>
              <img src={attachmentUrl(a.id)} alt={a.kind === "drawing" ? "Drawing" : "Image"} style={{ maxWidth: "100%", maxHeight: 260, borderRadius: 8, display: "block" }} />
              {canEdit && (
                <IconButton size="sm" variant="soft" aria-label="Remove image" onClick={() => remove(a)} sx={{ position: "absolute", top: 4, right: 4 }}>
                  <CloseIcon fontSize="small" />
                </IconButton>
              )}
            </Box>
          ))}
        </Box>
      )}
      {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
      {canEdit && (
        <Box sx={{ display: "inline-flex", gap: 0.25 }}>
          <input ref={file} type="file" hidden accept="image/png,image/jpeg,image/gif,image/webp" data-testid="note-image-input" onChange={(e) => { const f = e.target.files?.[0]; if (f) void addFile(f); e.target.value = ""; }} />
          <Tooltip title="Add image" size="sm">
            <IconButton size="sm" variant="plain" aria-label="Add image" data-testid="add-image" onClick={() => file.current?.click()}>
              <ImageOutlinedIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title="New drawing" size="sm">
            <IconButton size="sm" variant="plain" aria-label="New drawing" data-testid="add-drawing" onClick={() => setDrawing(true)}>
              <BrushOutlinedIcon />
            </IconButton>
          </Tooltip>
        </Box>
      )}
      {drawing && (
        <DrawingDialog
          onClose={() => setDrawing(false)}
          onSave={async (png) => {
            const ref = await uploadAttachment(noteId, "drawing", png);
            onChange([...items, ref]);
          }}
        />
      )}
    </Box>
  );
}
