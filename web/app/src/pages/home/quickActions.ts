import type { PaletteAction } from "./paletteLogic";

/** Quick actions offered on the home screen and in the command bar. Each one
 *  needs its app to be on for the user; `runQuickAction` does the work. */
export const QUICK_ACTIONS: PaletteAction[] = [
  { id: "new-doc", label: "New document", hint: "Docs", glyph: "docs", appId: "docs", keywords: ["doc", "write", "word", "letter"] },
  { id: "new-sheet", label: "New spreadsheet", hint: "Sheets", glyph: "sheets", appId: "sheets", keywords: ["sheet", "excel", "table", "budget"] },
  { id: "new-meeting", label: "Start a meeting", hint: "Meet", glyph: "meet", appId: "meet", keywords: ["meet", "call", "video", "meeting"] },
  { id: "new-invoice", label: "New invoice", hint: "Invoicing", glyph: "invoicing", appId: "invoicing", keywords: ["invoice", "bill", "quote"] },
  { id: "new-mail", label: "Write an email", hint: "Mail", glyph: "mail", appId: "mail", keywords: ["mail", "compose", "email", "send"] },
  { id: "new-event", label: "New event", hint: "Calendar", glyph: "calendar", appId: "calendar", keywords: ["event", "schedule", "calendar"] },
];

type Navigate = (to: string) => void;

/** runQuickAction creates the thing (documents and sheets are created first so
 *  the user lands inside the editor) or opens the app that makes it. */
export async function runQuickAction(id: string, navigate: Navigate): Promise<void> {
  switch (id) {
    case "new-doc": {
      const { createDoc } = await import("../docs/api");
      const d = await createDoc();
      navigate(`/docs/d/${d.id}`);
      return;
    }
    case "new-sheet": {
      const { createSheet } = await import("../sheets/api");
      const s = await createSheet();
      navigate(`/sheets/d/${s.id}`);
      return;
    }
    case "new-meeting":
      navigate("/meet");
      return;
    case "new-invoice":
      navigate("/invoicing");
      return;
    case "new-mail":
      navigate("/mail");
      return;
    case "new-event":
      navigate("/calendar");
      return;
  }
}
