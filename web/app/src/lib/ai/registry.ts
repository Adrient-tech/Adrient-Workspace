import { useEffect } from "react";
import type { AssistantApp, AssistantTool } from "./types";

const registry = new Map<string, Map<string, AssistantTool>>();
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

/** Tools currently registered for an app. */
export function getRegisteredTools(app: AssistantApp): AssistantTool[] {
  return [...(registry.get(app)?.values() ?? [])];
}

export function subscribeTools(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/** Non-hook registration; returns an unregister function. */
export function registerAssistantTools(app: AssistantApp, tools: AssistantTool[]): () => void {
  const m = registry.get(app) ?? new Map<string, AssistantTool>();
  registry.set(app, m);
  tools.forEach((t) => m.set(t.name, t));
  emit();
  return () => {
    tools.forEach((t) => {
      if (m.get(t.name) === t) m.delete(t.name);
    });
    emit();
  };
}

/**
 * Register the app's tools for the lifetime of the component.
 * Handlers run in the browser against the open document. Pass a memoised
 * array (or rely on handlers reading refs) so registration is not redone every render.
 */
export function useAssistantTools(app: AssistantApp, tools: AssistantTool[]): void {
  useEffect(() => registerAssistantTools(app, tools), [app, tools]);
}
