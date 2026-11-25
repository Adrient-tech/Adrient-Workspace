import type { AssistantApp } from "./types";

/** Default suggestion chips per app. Apps can override via props. */
export const DEFAULT_SUGGESTIONS: Record<string, string[]> = {
  sheets: [
    "Build a monthly budget with income, expenses and totals",
    "Explain the formula in the selected cell",
    "Summarise this sheet",
    "Add a chart of the selected data",
  ],
  docs: [
    "Summarise this document",
    "Rewrite the selection to be clearer",
    "Fix grammar and spelling",
    "Translate the selection to Hindi",
  ],
  slides: [
    "Outline a 6-slide pitch deck",
    "Rewrite this slide more concisely",
    "Generate speaker notes",
    "Suggest a better layout for this slide",
  ],
  global: [
    "What meetings do I have this week?",
    "Find the latest invoice from Acme",
    "Summarise my unread mail",
  ],
};

export function suggestionsFor(app: AssistantApp): string[] {
  return DEFAULT_SUGGESTIONS[app] ?? ["Summarise this", "What can you do here?"];
}
