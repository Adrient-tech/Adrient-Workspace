import { compressOnServer } from "../editor/serverTools";
import { useRef, useState } from "react";
import { pdfjs } from "react-pdf";
import pdfWorkerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import {
  chunkPages, diffSummary, diffWords, extractPages, humanSize, imagesToPdf, optimizePdf, pageCountOf, sizeChange, splitGroups,
  type DiffPart,
} from "./pdfTools";

pdfjs.GlobalWorkerOptions.workerSrc = pdfWorkerUrl;

type Tab = "compress" | "split" | "compare";

function download(bytes: Uint8Array, name: string) {
  const blob = new Blob([bytes as BlobPart], { type: "application/pdf" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 5000);
}

const baseName = (f: File) => f.name.replace(/\.pdf$/i, "");
const readBytes = async (f: File) => new Uint8Array(await f.arrayBuffer());

function FilePick({ label, file, onFile, testId }: { label: string; file: File | null; onFile: (f: File | null) => void; testId: string }) {
  const ref = useRef<HTMLInputElement>(null);
  return (
    <div className="flex items-center gap-3 flex-wrap">
      <input ref={ref} type="file" accept="application/pdf,.pdf" hidden data-testid={testId} onChange={(e) => onFile(e.target.files?.[0] ?? null)} />
      <button type="button" className="px-3 py-1.5 rounded-md border border-border bg-surface hover:bg-background text-sm" onClick={() => ref.current?.click()}>
        {label}
      </button>
      <span className="text-sm text-text-muted truncate max-w-xs">{file ? `${file.name} (${humanSize(file.size)})` : "No file chosen"}</span>
    </div>
  );
}

const btn = "px-3 py-1.5 rounded-md bg-primary text-white text-sm disabled:opacity-50";

// ---------------------------------------------------------------- compress

function Compress() {
  const [file, setFile] = useState<File | null>(null);
  const [mode, setMode] = useState<"optimize" | "flatten">("optimize");
  const [strip, setStrip] = useState(false);
  const [quality, setQuality] = useState(0.6);
  const [busy, setBusy] = useState("");
  const [err, setErr] = useState("");
  const [result, setResult] = useState<{ before: number; after: number; name: string } | null>(null);

  async function run() {
    if (!file) return;
    setErr("");
    setResult(null);
    try {
      const bytes = await readBytes(file);
      let out: Uint8Array;
      if (mode === "optimize") {
        setBusy("Optimising...");
        // Our server removes duplicate fonts and images and recompresses
        // streams; the in-browser pass then strips metadata if asked, and is
        // the whole job when the server is unavailable.
        const server = await compressOnServer(bytes);
        out = server && !strip ? server : (await optimizePdf(server ?? bytes, strip)).bytes;
        if (out.byteLength > bytes.byteLength) out = bytes;
      } else {
        setBusy("Rendering pages...");
        const pdf = await pdfjs.getDocument({ data: bytes.slice() }).promise;
        const pages: Parameters<typeof imagesToPdf>[0] = [];
        for (let n = 1; n <= pdf.numPages; n++) {
          setBusy(`Rendering page ${n} of ${pdf.numPages}...`);
          const page = await pdf.getPage(n);
          const base = page.getViewport({ scale: 1 });
          const vp = page.getViewport({ scale: Math.min(2, 1400 / Math.max(base.width, base.height)) });
          const canvas = document.createElement("canvas");
          canvas.width = Math.ceil(vp.width);
          canvas.height = Math.ceil(vp.height);
          await page.render({ canvasContext: canvas.getContext("2d")!, viewport: vp, canvas }).promise;
          const blob = await new Promise<Blob>((res, rej) => canvas.toBlob((b) => (b ? res(b) : rej(new Error("Could not render a page"))), "image/jpeg", quality));
          pages.push({ bytes: new Uint8Array(await blob.arrayBuffer()), type: "jpeg", width: base.width, height: base.height });
        }
        out = await imagesToPdf(pages);
      }
      const name = `${baseName(file)}-${mode === "optimize" ? "optimized" : "compressed"}.pdf`;
      setResult({ before: bytes.byteLength, after: out.byteLength, name });
      download(out, name);
    } catch (e) {
      setErr((e as Error).message || "Could not process that file. It may be damaged or password protected.");
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="space-y-4" data-testid="tool-compress">
      <p className="text-sm text-text-muted">Make a PDF smaller. Optimise keeps text selectable and searchable; it helps most with files that carry a lot of structure overhead. Shrink pages turns each page into a picture, which usually gives the smallest file, but text can no longer be selected or searched.</p>
      <FilePick label="Choose a PDF" file={file} onFile={(f) => { setFile(f); setResult(null); }} testId="compress-file" />
      <div className="flex gap-4 flex-wrap text-sm">
        <label className="flex items-center gap-2"><input type="radio" checked={mode === "optimize"} onChange={() => setMode("optimize")} />Optimise (keeps text)</label>
        <label className="flex items-center gap-2"><input type="radio" checked={mode === "flatten"} onChange={() => setMode("flatten")} data-testid="mode-flatten" />Shrink pages (text becomes a picture)</label>
      </div>
      {mode === "optimize" ? (
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={strip} onChange={(e) => setStrip(e.target.checked)} />Remove title, author and other document details</label>
      ) : (
        <label className="flex items-center gap-2 text-sm">Picture quality
          <input type="range" min={0.3} max={0.9} step={0.05} value={quality} onChange={(e) => setQuality(Number(e.target.value))} />
          <span>{Math.round(quality * 100)}%</span>
        </label>
      )}
      <button className={btn} disabled={!file || !!busy} onClick={run} data-testid="compress-run">{busy || "Compress and download"}</button>
      {err && <p className="text-sm text-red-600" data-testid="tool-error">{err}</p>}
      {result && (
        <p className="text-sm" data-testid="compress-result">
          {humanSize(result.before)} to {humanSize(result.after)} ({sizeChange(result.before, result.after)}). Saved as {result.name}.
          {result.after >= result.before && (mode === "optimize" ? " This file was already compact. Shrink pages may make it smaller if it holds large images." : " The pages as pictures are larger than the original, so keep the original.")}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- split

function Split() {
  const [file, setFile] = useState<File | null>(null);
  const [pages, setPages] = useState<number | null>(null);
  const [mode, setMode] = useState<"ranges" | "every">("ranges");
  const [spec, setSpec] = useState("");
  const [every, setEvery] = useState(1);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [done, setDone] = useState("");

  async function pick(f: File | null) {
    setFile(f);
    setPages(null);
    setDone("");
    setErr("");
    if (!f) return;
    try {
      setPages(await pageCountOf(await readBytes(f)));
    } catch {
      setErr("Could not read that file. It may be damaged or password protected.");
    }
  }
  async function run() {
    if (!file || pages === null) return;
    setErr("");
    setDone("");
    setBusy(true);
    try {
      const groups = mode === "ranges" ? splitGroups(spec, pages) : chunkPages(pages, every);
      const bytes = await readBytes(file);
      let n = 0;
      for (const g of groups) {
        n++;
        const first = g[0] + 1;
        const last = g[g.length - 1] + 1;
        download(await extractPages(bytes, g), `${baseName(file)}-${g.length === 1 ? `page-${first}` : `pages-${first}-${last}`}.pdf`);
        await new Promise((r) => setTimeout(r, 150)); // let the browser start each download
      }
      setDone(`Created ${n} file${n === 1 ? "" : "s"}.`);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-4" data-testid="tool-split">
      <p className="text-sm text-text-muted">Cut a PDF into several files, or pull out the pages you need.</p>
      <FilePick label="Choose a PDF" file={file} onFile={pick} testId="split-file" />
      {pages !== null && <p className="text-sm">{pages} page{pages === 1 ? "" : "s"}.</p>}
      <div className="flex gap-4 flex-wrap text-sm">
        <label className="flex items-center gap-2"><input type="radio" checked={mode === "ranges"} onChange={() => setMode("ranges")} />By page ranges</label>
        <label className="flex items-center gap-2"><input type="radio" checked={mode === "every"} onChange={() => setMode("every")} data-testid="split-mode-every" />Every N pages</label>
      </div>
      {mode === "ranges" ? (
        <div>
          <input className="border border-border rounded-md px-2 py-1.5 text-sm w-full max-w-md bg-surface" placeholder="1-3, 4-6, 9" value={spec} onChange={(e) => setSpec(e.target.value)} data-testid="split-ranges" aria-label="Page ranges" />
          <p className="text-xs text-text-muted mt-1">Each range becomes its own file. Use "1-3; 5" or commas to separate them.</p>
        </div>
      ) : (
        <label className="flex items-center gap-2 text-sm">Pages per file <input type="number" min={1} value={every} onChange={(e) => setEvery(Number(e.target.value))} className="border border-border rounded-md px-2 py-1 w-20 bg-surface" data-testid="split-every" /></label>
      )}
      <button className={btn} disabled={!file || pages === null || busy} onClick={run} data-testid="split-run">{busy ? "Working..." : "Split and download"}</button>
      {err && <p className="text-sm text-red-600" data-testid="tool-error">{err}</p>}
      {done && <p className="text-sm" data-testid="split-done">{done}</p>}
    </div>
  );
}

// ---------------------------------------------------------------- compare

async function pageTexts(bytes: Uint8Array): Promise<string[]> {
  const pdf = await pdfjs.getDocument({ data: bytes.slice() }).promise;
  const out: string[] = [];
  for (let n = 1; n <= pdf.numPages; n++) {
    const tc = await (await pdf.getPage(n)).getTextContent();
    out.push(tc.items.map((it) => ("str" in it ? it.str : "")).join(" "));
  }
  return out;
}

function Compare() {
  const [a, setA] = useState<File | null>(null);
  const [b, setB] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [res, setRes] = useState<{ pages: { n: number; parts: DiffPart[]; changed: boolean }[]; total: { added: number; removed: number; same: number }; pa: number; pb: number } | null>(null);

  async function run() {
    if (!a || !b) return;
    setBusy(true);
    setErr("");
    setRes(null);
    try {
      const [ta, tb] = await Promise.all([pageTexts(await readBytes(a)), pageTexts(await readBytes(b))]);
      const n = Math.max(ta.length, tb.length);
      const pages: { n: number; parts: DiffPart[]; changed: boolean }[] = [];
      const total = { added: 0, removed: 0, same: 0 };
      for (let i = 0; i < n; i++) {
        const parts = diffWords(ta[i] ?? "", tb[i] ?? "");
        const s = diffSummary(parts);
        total.added += s.added;
        total.removed += s.removed;
        total.same += s.same;
        pages.push({ n: i + 1, parts, changed: s.added + s.removed > 0 });
      }
      setRes({ pages, total, pa: ta.length, pb: tb.length });
      if (ta.join("").trim() === "" && tb.join("").trim() === "") setErr("These files have no selectable text (they may be scans), so there is nothing to compare.");
    } catch (e) {
      setErr((e as Error).message || "Could not read one of the files.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-4" data-testid="tool-compare">
      <p className="text-sm text-text-muted">Find what changed between two versions of a document. The comparison reads the text, page by page. Scanned pages without text cannot be compared.</p>
      <FilePick label="Original" file={a} onFile={setA} testId="compare-a" />
      <FilePick label="Revised" file={b} onFile={setB} testId="compare-b" />
      <button className={btn} disabled={!a || !b || busy} onClick={run} data-testid="compare-run">{busy ? "Comparing..." : "Compare"}</button>
      {err && <p className="text-sm text-red-600" data-testid="tool-error">{err}</p>}
      {res && (
        <div data-testid="compare-result">
          <p className="text-sm mb-2"><b>{res.total.removed}</b> words removed, <b>{res.total.added}</b> words added, {res.total.same} unchanged. {res.pa !== res.pb ? `Page count changed from ${res.pa} to ${res.pb}.` : ""}</p>
          {res.pages.filter((p) => p.changed).length === 0 && <p className="text-sm" data-testid="compare-same">No differences in text.</p>}
          {res.pages.filter((p) => p.changed).map((p) => (
            <div key={p.n} className="border border-border rounded-md p-3 mb-3 bg-surface">
              <div className="text-xs text-text-muted mb-1">Page {p.n}</div>
              <p className="text-sm leading-relaxed">
                {p.parts.map((x, i) =>
                  x.kind === "same" ? <span key={i}>{x.text} </span>
                    : x.kind === "added" ? <mark key={i} className="bg-green-200 text-green-900 px-0.5 rounded" data-kind="added">{x.text} </mark>
                    : <del key={i} className="bg-red-200 text-red-900 px-0.5 rounded" data-kind="removed">{x.text} </del>,
                )}
              </p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function ToolsPage() {
  const [tab, setTab] = useState<Tab>("compress");
  const tabs: { id: Tab; label: string }[] = [
    { id: "compress", label: "Compress" },
    { id: "split", label: "Split" },
    { id: "compare", label: "Compare" },
  ];
  return (
    <div className="max-w-3xl mx-auto p-4 sm:p-6">
      <h1 className="text-xl font-semibold mb-1">PDF tools</h1>
      <p className="text-sm text-text-muted mb-4">Files are processed in your browser and are not uploaded. To redact, fill forms, sign, merge, rotate or reorder pages, open the PDF editor.</p>
      <div role="tablist" className="flex gap-1 border-b border-border mb-4 overflow-x-auto">
        {tabs.map((t) => (
          <button key={t.id} role="tab" aria-selected={tab === t.id} data-testid={`tab-${t.id}`} onClick={() => setTab(t.id)} className={`px-4 py-2 text-sm -mb-px border-b-2 ${tab === t.id ? "border-primary text-primary font-medium" : "border-transparent text-text-muted hover:text-text"}`}>
            {t.label}
          </button>
        ))}
      </div>
      {tab === "compress" && <Compress />}
      {tab === "split" && <Split />}
      {tab === "compare" && <Compare />}
    </div>
  );
}
