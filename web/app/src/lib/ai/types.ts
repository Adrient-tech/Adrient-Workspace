/**
 * Public contract of the Adrient AI assistant (see docs/ai.md).
 * App code (Sheets, Docs, Slides, ...) imports ONLY from "@/lib/ai".
 */

/** Apps that can host the assistant. */
export type AssistantApp =
  | "sheets" | "docs" | "slides" | "drive" | "mail" | "calendar"
  | "contacts" | "chat" | "forms" | "global" | (string & {});

/** JSON-Schema subset used to describe tool arguments. */
export interface JSONSchema {
  type?: "object" | "string" | "number" | "integer" | "boolean" | "array";
  description?: string;
  properties?: Record<string, JSONSchema>;
  required?: string[];
  items?: JSONSchema;
  enum?: (string | number)[];
  additionalProperties?: boolean;
}

/**
 * read: no change to the document, runs immediately.
 * write: changes the document, runs immediately, shown with Undo.
 * destructive: asks the user to confirm in the panel before running
 *   (delete, overwrite of non-empty content, anything hard to undo).
 */
export type ToolRisk = "read" | "write" | "destructive";

export interface ToolOutcome {
  /** JSON-serialisable result returned to the model. Keep it small. */
  result?: unknown;
  /** Set when the tool failed; the model sees it and may retry. */
  error?: string;
  /** Reverts exactly this call. Required for write/destructive tools that can be undone. */
  undo?: () => void | Promise<void>;
  /** One-line human summary shown in the panel ("Wrote 12 cells in B2:D5"). */
  summary?: string;
}

export interface AssistantTool {
  /** snake_case, unique inside the app, e.g. "sheet_write_cells". */
  name: string;
  /** What it does and when to use it (the model reads this). */
  description: string;
  parameters: JSONSchema;
  risk: ToolRisk;
  /** Runs in the browser against the open document. Must validate its own args. */
  handler: (args: Record<string, unknown>) => ToolOutcome | Promise<ToolOutcome>;
  /** Optional human label for the confirmation / activity row. */
  label?: (args: Record<string, unknown>) => string;
}

/** What the user currently has selected, shown as a chip and sent as context. */
export interface AssistantContext {
  /** Short label for the chip, e.g. "Sheet1!A1:C4". */
  label: string;
  /** Text sent to the model, treated as DATA. Keep it bounded (the panel truncates to 8000 chars). */
  text?: string;
}

export interface AssistantPanelProps {
  app: AssistantApp;
  /** Stable id of the open document; chat history is kept per app+document. */
  documentId: string;
  documentTitle?: string;
  /** Tools for this panel. If omitted the tools registered via useAssistantTools(app) are used. */
  tools?: AssistantTool[];
  /** Returns the current selection. Called when a message is sent and when the chip refreshes. */
  getContext?: () => AssistantContext | null;
  /** Subscribe to selection changes so the chip updates live; return an unsubscribe. */
  subscribeContext?: (cb: () => void) => () => void;
  /** Extra app-specific guidance appended to the system prompt (document data rules still apply). */
  systemHint?: string;
  /** Suggestion chips; defaults per app. */
  suggestions?: string[];
  onClose?: () => void;
  /** Shows an "Insert" button on answers; receives the answer's Markdown text. */
  onInsertText?: (markdown: string) => void;
}

// ---- wire protocol (server <-> browser), see docs/ai.md -------------------

export interface WireToolDef {
  name: string;
  description: string;
  parameters: JSONSchema;
  /** The server drops write/destructive tools for users who can only view the document. */
  risk: ToolRisk;
}
export interface WireToolCall {
  id: string;
  name: string;
  arguments: string; // JSON text
}
export interface WireMessage {
  role: "user" | "assistant" | "tool";
  content: string;
  tool_calls?: WireToolCall[];
  tool_call_id?: string;
}
export interface ChatRequest {
  app: string;
  document_id?: string;
  model?: string;
  messages: WireMessage[];
  tools?: WireToolDef[];
  /** Selection / document text. The server wraps it as untrusted data. */
  context?: { label: string; text?: string };
  system_hint?: string;
  /** Text files the user attached (from Drive). The server wraps them as untrusted data. */
  attachments?: { name: string; text: string }[];
}
export type StreamEvent =
  | { type: "delta"; text: string }
  | { type: "tool_call"; call: WireToolCall }
  | { type: "source"; title: string; url?: string; app?: string }
  | { type: "usage"; input_tokens: number; output_tokens: number }
  | { type: "error"; code: string; message: string }
  | { type: "done"; finish: string };
