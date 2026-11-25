// Calls to the PDF tools that run on our own Go server (no third-party
// service): true content removal, compression and the per-user signature
// store. Every call fails soft (returns null) so the editor can fall back to
// its in-browser path when the server tool is unavailable.
import { API_BASE } from "@/utils/apiClient";

/** A region in PDF default user space (points, origin bottom-left). */
export interface PdfRegion {
  page: number; // 1-based
  x: number;
  y: number;
  w: number;
  h: number;
  /** Text edit: remove only glyphs, keep drawings and images. */
  glyphsOnly?: boolean;
}

export interface RemoveStats {
  page: number;
  glyphsRemoved: number;
  imagesRemoved: number;
  imagesClipped: number;
  pathsRemoved: number;
  unhandled: number;
}

/** A normalized top-left box (0-1 of the page, y down). */
export interface NormBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

/**
 * Convert a normalized top-left box to PDF user space for a page whose
 * MediaBox is (mx, my, mw, mh). Pages are treated as unrotated, matching how
 * the editor stores annotation coordinates.
 */
export function normToPdfRegion(page: number, b: NormBox, media: { x: number; y: number; width: number; height: number }): PdfRegion {
  return {
    page,
    x: media.x + b.x * media.width,
    y: media.y + (1 - b.y - b.h) * media.height,
    w: b.w * media.width,
    h: b.h * media.height,
  };
}

const headers = { "X-Requested-With": "pdf-frontend" };

export async function removeContentOnServer(
  bytes: Uint8Array,
  regions: PdfRegion[],
): Promise<{ bytes: Uint8Array; stats: RemoveStats[] } | null> {
  if (!regions.length) return null;
  try {
    const res = await fetch(`${API_BASE}/tools/remove-content?regions=${encodeURIComponent(JSON.stringify(regions))}`, {
      method: "POST",
      credentials: "include",
      headers: { ...headers, "Content-Type": "application/pdf" },
      body: new Blob([bytes as BlobPart], { type: "application/pdf" }),
    });
    if (!res.ok) return null;
    const stats = JSON.parse(res.headers.get("X-Removed-Content") || "[]") as RemoveStats[];
    return { bytes: new Uint8Array(await res.arrayBuffer()), stats: stats ?? [] };
  } catch {
    return null;
  }
}

export async function compressOnServer(bytes: Uint8Array): Promise<Uint8Array | null> {
  try {
    const res = await fetch(`${API_BASE}/tools/compress`, {
      method: "POST",
      credentials: "include",
      headers: { ...headers, "Content-Type": "application/pdf" },
      body: new Blob([bytes as BlobPart], { type: "application/pdf" }),
    });
    if (!res.ok) return null;
    return new Uint8Array(await res.arrayBuffer());
  } catch {
    return null;
  }
}

export interface SavedSignature {
  id: string;
  kind: "signature" | "initials";
  label: string;
  imagePng: string;
  createdAt: string;
}

export async function listSavedSignatures(): Promise<SavedSignature[] | null> {
  try {
    const res = await fetch(`${API_BASE}/user/signatures`, { credentials: "include", headers });
    if (!res.ok) return null;
    const j = (await res.json()) as { signatures?: SavedSignature[] };
    return j.signatures ?? [];
  } catch {
    return null;
  }
}

export async function saveSignature(kind: "signature" | "initials", imagePng: string, label = ""): Promise<SavedSignature | null> {
  try {
    const res = await fetch(`${API_BASE}/user/signatures`, {
      method: "POST",
      credentials: "include",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify({ kind, imagePng, label }),
    });
    if (!res.ok) return null;
    return (await res.json()) as SavedSignature;
  } catch {
    return null;
  }
}

export async function deleteSavedSignature(id: string): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/user/signatures/${encodeURIComponent(id)}`, {
      method: "DELETE",
      credentials: "include",
      headers,
    });
    return res.ok;
  } catch {
    return false;
  }
}

export interface PageFont {
  resource: string;
  baseFont: string;
  subtype: string;
  bold: boolean;
  italic: boolean;
  format?: "truetype" | "opentype";
  file?: Uint8Array;
}

/** Strip a subset tag ("ABCDEF+Name") and normalise for matching. */
export function fontKey(name: string): string {
  return name.replace(/^[A-Z]{6}\+/, "").replace(/[\s_-]/g, "").toLowerCase();
}

const pageFontCache = new WeakMap<Uint8Array, Map<number, Promise<PageFont[] | null>>>();

/** Fonts of one page (1-based) with embedded programs, cached per file. */
export function pageFontsOnServer(bytes: Uint8Array, page: number): Promise<PageFont[] | null> {
  let m = pageFontCache.get(bytes);
  if (!m) pageFontCache.set(bytes, (m = new Map()));
  const hit = m.get(page);
  if (hit) return hit;
  const p = (async () => {
    try {
      const res = await fetch(`${API_BASE}/tools/page-fonts?page=${page}`, {
        method: "POST",
        credentials: "include",
        headers: { ...headers, "Content-Type": "application/pdf" },
        body: new Blob([bytes as BlobPart], { type: "application/pdf" }),
      });
      if (!res.ok) return null;
      const j = (await res.json()) as { fonts?: (Omit<PageFont, "file"> & { file?: string })[] };
      return (j.fonts ?? []).map((f) => ({
        ...f,
        file: f.file ? Uint8Array.from(atob(f.file), (c) => c.charCodeAt(0)) : undefined,
      }));
    } catch {
      return null;
    }
  })();
  m.set(page, p);
  return p;
}

/** Pick the embedded font matching a the renderer font name, if the file carries one. */
export function matchEmbeddedFont(fonts: PageFont[] | null, pdfjsName: string | undefined): PageFont | null {
  if (!fonts || !pdfjsName) return null;
  const k = fontKey(pdfjsName);
  return fonts.find((f) => f.file && fontKey(f.baseFont) === k) ?? null;
}
