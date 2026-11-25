import { useEffect, useRef, useState } from "react";
import { Box, CircularProgress, Typography } from "@mui/joy";
import * as pdfjsLib from "pdfjs-dist";
import workerSrc from "pdfjs-dist/build/pdf.worker.min.mjs?url";

pdfjsLib.GlobalWorkerOptions.workerSrc = workerSrc;

export interface PdfPagesProps {
  url: string;
  /** Called with the number of pages once the document is open. */
  onLoaded?: (pages: number) => void;
  /** Content drawn over page n (1-based). It fills the page box; use percentages for positions. */
  overlay?: (page: number) => React.ReactNode;
  /** Click on a page (fractions from the top-left corner). */
  onPageClick?: (page: number, x: number, y: number) => void;
  maxWidth?: number;
}

/** Draws every page of a PDF at the width of its container, with an overlay layer per page. */
export function PdfPages({ url, onLoaded, overlay, onPageClick, maxWidth = 820 }: PdfPagesProps) {
  const [pages, setPages] = useState<{ n: number; ratio: number }[] | null>(null);
  const [err, setErr] = useState("");
  const docRef = useRef<pdfjsLib.PDFDocumentProxy | null>(null);
  const [ver, setVer] = useState(0);
  useEffect(() => {
    let dead = false;
    setPages(null);
    setErr("");
    const task = pdfjsLib.getDocument({ url, withCredentials: true });
    task.promise
      .then(async (doc) => {
        if (dead) return;
        docRef.current = doc;
        const list: { n: number; ratio: number }[] = [];
        for (let i = 1; i <= doc.numPages; i++) {
          const v = (await doc.getPage(i)).getViewport({ scale: 1 });
          list.push({ n: i, ratio: v.height / v.width });
        }
        if (dead) return;
        setPages(list);
        setVer((x) => x + 1);
        onLoaded?.(doc.numPages);
      })
      .catch((e: Error) => !dead && setErr(e.message));
    return () => { dead = true; void task.destroy(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url]);
  if (err) return <Typography color="danger" role="alert" sx={{ p: 2 }}>The document could not be shown: {err}</Typography>;
  if (!pages) return <Box sx={{ p: 4, textAlign: "center" }}><CircularProgress /></Box>;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2, alignItems: "center" }} data-testid="pdf-pages" data-ver={ver}>
      {pages.map((p) => (
        <PageCanvas key={p.n} n={p.n} ratio={p.ratio} doc={docRef.current!} maxWidth={maxWidth} overlay={overlay?.(p.n)} onClick={onPageClick} />
      ))}
    </Box>
  );
}

function PageCanvas({ n, ratio, doc, maxWidth, overlay, onClick }: { n: number; ratio: number; doc: pdfjsLib.PDFDocumentProxy; maxWidth: number; overlay?: React.ReactNode; onClick?: PdfPagesProps["onPageClick"] }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const box = useRef<HTMLDivElement>(null);
  const [w, setW] = useState(maxWidth);
  useEffect(() => {
    const el = box.current?.parentElement;
    if (!el) return;
    const ro = new ResizeObserver(() => setW(Math.min(maxWidth, Math.max(240, el.clientWidth - 2))));
    ro.observe(el);
    setW(Math.min(maxWidth, Math.max(240, el.clientWidth - 2)));
    return () => ro.disconnect();
  }, [maxWidth]);
  useEffect(() => {
    let dead = false;
    void doc.getPage(n).then((page) => {
      if (dead || !ref.current) return;
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      const base = page.getViewport({ scale: 1 });
      const viewport = page.getViewport({ scale: (w / base.width) * dpr });
      const c = ref.current;
      c.width = viewport.width;
      c.height = viewport.height;
      const ctx = c.getContext("2d");
      if (ctx) void page.render({ canvasContext: ctx, viewport }).promise.catch(() => undefined);
    });
    return () => { dead = true; };
  }, [doc, n, w]);
  return (
    <Box ref={box} sx={{ position: "relative", width: w, height: w * ratio, boxShadow: "0 2px 10px rgba(0,0,0,.18)", bgcolor: "#fff", cursor: onClick ? "crosshair" : "default" }} data-page={n}
      onClick={onClick ? (e) => { const r = (e.currentTarget as HTMLElement).getBoundingClientRect(); onClick(n, (e.clientX - r.left) / r.width, (e.clientY - r.top) / r.height); } : undefined}>
      <canvas ref={ref} style={{ width: "100%", height: "100%", display: "block" }} />
      {overlay}
    </Box>
  );
}
