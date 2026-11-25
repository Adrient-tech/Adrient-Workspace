import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import type { Plugin } from "vite";
import { fileURLToPath } from "node:url";
import { execSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";

// Version source of truth: the repo-root VERSION file (also embedded in the Go
// binaries). Falls back to ADRIENT_VERSION, then "dev". The build stamp is the
// date plus the short git sha when git is available.
const here = fileURLToPath(new URL(".", import.meta.url));
function readVersion(): string {
  for (const f of [resolve(here, "../../VERSION"), resolve(here, "../VERSION")]) {
    if (existsSync(f)) return readFileSync(f, "utf8").trim();
  }
  return process.env.ADRIENT_VERSION || "dev";
}
function buildStamp(): string {
  const date = new Date().toISOString().slice(0, 10);
  try {
    const sha = execSync("git rev-parse --short HEAD", { cwd: here, stdio: ["ignore", "pipe", "ignore"] }).toString().trim();
    return sha ? `${date} · ${sha}` : date;
  } catch {
    return process.env.ADRIENT_COMMIT ? `${date} · ${process.env.ADRIENT_COMMIT.slice(0, 7)}` : date;
  }
}

// FortuneSheet 1.0.4 paints every cell without an explicit horizontal
// alignment (`ht`) left-aligned, numbers included. Excel and Google Sheets
// right-align numbers, centre booleans and errors, and left-align text. Patch
// the one resolver FortuneSheet paints with (`normalizedCellAttr(cell, "ht")`)
// to ask src/pages/sheets/cellAlign.ts for the type default. It is decided at
// render time and never written into cells, so saves and the xlsx export
// keep "no alignment" for cells that had none. Fails the build if the
// library's code changes shape (e.g. after an upgrade).
const FS_HT_FROM = 'var defaultValue = attr === "ht" ? "1" : "0";';
const FS_HT_TO =
  'var defaultValue = attr === "ht" ? (typeof globalThis.__adrientDefaultHt === "function" ? globalThis.__adrientDefaultHt(cell) : "1") : "0";';
// setCaretPosition's catch block dereferences formulaCache.rangeResizeTo[0],
// which is undefined while typing a formula, so the handler itself threw and
// the editor stopped taking input. Fall back to the input element.
const FS_CARET_FROM = "moveToEnd(ctx.formulaCache.rangeResizeTo[0]);";
const FS_CARET_TO =
  "try { var __rr = ctx.formulaCache && ctx.formulaCache.rangeResizeTo; moveToEnd(__rr && __rr[0] ? __rr[0] : textDom); } catch (e2) { console.error(e2); }";
// The React wrapper's setSelection API runs api.setSelection inside an immer
// recipe with the caller's range objects. FortuneSheet's normalizeSelection
// writes into those objects, and immer freezes them once they are part of the
// state, so a replayed recipe threw "Cannot assign to read only property
// 'row'" and the whole editor unmounted (blank page; seen with Ctrl+A). Give
// every run its own copy.
const FS_SEL_FROM = /return ((?:core\.)?api)\.setSelection\(draftCtx, range, options\);/;
const FS_SEL_TO = "return $1.setSelection(draftCtx, JSON.parse(JSON.stringify(range)), options);";
const isFortuneCore = (id: string) => /@fortune-sheet[\\/]core[\\/]dist[\\/]index(\.esm)?\.js$/.test(id.split("?")[0]);
const isFortuneReact = (id: string) => /@fortune-sheet[\\/]react[\\/]dist[\\/]index(\.esm)?\.js$/.test(id.split("?")[0]);
function replaceOnce(code: string, from: string | RegExp, to: string, what: string, id: string): string {
  if (typeof from === "string" ? !code.includes(from) : !from.test(code)) throw new Error(`fortune-sheet ${what} patch: pattern not found in ${id}`);
  return code.replace(from, to);
}
function patchFortuneCore(code: string, id: string): string {
  code = replaceOnce(code, FS_HT_FROM, FS_HT_TO, "alignment", id);
  return replaceOnce(code, FS_CARET_FROM, FS_CARET_TO, "caret", id);
}
function patchFortuneReact(code: string, id: string): string {
  return replaceOnce(code, FS_SEL_FROM, FS_SEL_TO, "selection", id);
}
function patchFortune(code: string, id: string): string | null {
  if (isFortuneCore(id)) return patchFortuneCore(code, id);
  if (isFortuneReact(id)) return patchFortuneReact(code, id);
  return null;
}
function fortuneSheetAlignment(): Plugin {
  return {
    name: "adrient:fortune-sheet-alignment",
    enforce: "pre",
    transform(code, id) {
      const out = patchFortune(code, id);
      return out == null ? null : { code: out, map: null };
    },
  };
}

export default defineConfig({
  define: {
    __ADRIENT_VERSION__: JSON.stringify(readVersion()),
    __ADRIENT_BUILD__: JSON.stringify(buildStamp()),
  },
  plugins: [react(), fortuneSheetAlignment()],
  // "@/..." resolves to src/ (used by the AI assistant contract, docs/ai.md).
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  // The dev server pre-bundles dependencies with esbuild, bypassing
  // `transform`; patch FortuneSheet there too.
  optimizeDeps: {
    esbuildOptions: {
      plugins: [
        {
          name: "adrient:fortune-sheet-alignment",
          setup(build) {
            build.onLoad({ filter: /@fortune-sheet[\\/](core|react)[\\/]dist[\\/]index(\.esm)?\.js$/ }, async (args) => {
              const { readFile } = await import("node:fs/promises");
              const code = await readFile(args.path, "utf8");
              return { contents: patchFortune(code, args.path) ?? code, loader: "js" };
            });
          },
        },
      ],
    },
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    proxy: {
      // In dev, proxy /api and /healthz to the Go backend on 8080.
      // Production: backend serves both the API and the built SPA directly.
      // ws: the collab hubs (/api/v1/{docs,sheets,slides,…}/d/<id>/connect)
      // are WebSockets; without this the dev editors silently run offline.
      "/api": { target: "http://127.0.0.1:8080", ws: true },
      "/healthz": "http://127.0.0.1:8080",
    },
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
  },
});
