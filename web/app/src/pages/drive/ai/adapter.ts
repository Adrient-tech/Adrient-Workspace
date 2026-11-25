import { search } from "../../../api/client";
import { loadDriveText } from "@/lib/ai/attachments";
import { createFolder, getFile, listFiles, moveFile, trashFile } from "../api";
import type { DriveAdapter } from "./tools";

const APP: Record<string, string> = {
  SEARCH_RESULT_TYPE_DRIVE: "drive", SEARCH_RESULT_TYPE_DOCS: "docs", SEARCH_RESULT_TYPE_SHEETS: "sheets", SEARCH_RESULT_TYPE_SLIDES: "slides",
  SEARCH_RESULT_TYPE_CONTACTS: "contacts", SEARCH_RESULT_TYPE_KEEP: "keep", SEARCH_RESULT_TYPE_CALENDAR: "calendar", SEARCH_RESULT_TYPE_MAIL: "mail",
};

/** The real adapter: the user's own session against /api/v1/drive. */
export function driveAdapter(h: { currentFolder: () => string; changed: () => void }): DriveAdapter {
  const after = async <T,>(p: Promise<T>) => {
    const r = await p;
    h.changed();
    return r;
  };
  return {
    list: listFiles,
    get: getFile,
    search: async (q) => {
      const r = await search(q, 15);
      return r.groups.flatMap((g) => g.results.map((x) => ({ app: APP[x.type] ?? "other", id: x.id, title: x.title, snippet: x.snippet, url: x.url })));
    },
    readText: async (f) => (await loadDriveText(f)).text,
    createFolder: (n, p) => after(createFolder(n, p)),
    move: async (id, p) => void (await after(moveFile(id, p))),
    trash: (id) => after(trashFile(id)),
    currentFolder: h.currentFolder,
  };
}
