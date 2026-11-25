import { useSyncExternalStore } from "react";

// Tiny open/close store for the global command bar so the top bar, the home
// screen and the Ctrl-K handler can all open the same instance.
let open = false;
let initial = "";
const subs = new Set<() => void>();

function emit() {
  subs.forEach((s) => s());
}

export function openPalette(query = "") {
  open = true;
  initial = query;
  emit();
}

export function closePalette() {
  open = false;
  emit();
}

export function usePalette(): { open: boolean; initial: string } {
  const isOpen = useSyncExternalStore(
    (cb) => {
      subs.add(cb);
      return () => {
        subs.delete(cb);
      };
    },
    () => open,
    () => false,
  );
  return { open: isOpen, initial };
}

/** isCommandKey: Ctrl-K or Cmd-K. */
export function isCommandKey(e: Pick<KeyboardEvent, "key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey">): boolean {
  return (e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k";
}

/** Ctrl-K stays with rich-text editors (it inserts a link there). */
export function shouldIgnoreCommandKey(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || typeof el.closest !== "function") return false;
  return !!el.closest('[contenteditable="true"], .ProseMirror, .ql-editor');
}
