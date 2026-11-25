// Pure helpers behind the PersonCard buttons: which URL each action opens and
// how the action is delivered when the target app is already on screen.

export interface PersonTarget {
  id?: string;
  name?: string;
  email?: string;
}

export type PersonAction = "chat" | "call" | "email" | "schedule";

/** App URL opened by an action ("call" has none: it creates a Meet room). */
export function actionHref(action: PersonAction, p: PersonTarget): string | null {
  switch (action) {
    case "chat":
      return p.id ? `/chat?dm=${encodeURIComponent(p.id)}` : null;
    case "email":
      return p.email ? `/mail?compose=1&to=${encodeURIComponent(p.email)}` : null;
    case "schedule":
      return p.email
        ? `/calendar?new=1&attendee=${encodeURIComponent(p.email)}`
        : null;
    default:
      return null;
  }
}

/** Which action buttons are usable for this target. */
export function availableActions(p: PersonTarget): PersonAction[] {
  const out: PersonAction[] = [];
  if (p.id) out.push("chat", "call");
  if (p.email) out.push("email", "schedule");
  return out;
}

export interface ActionDeps {
  navigate: (to: string) => void;
  /** Current location.pathname (to detect that the target app is open). */
  pathname: string;
  /** Creates a Meet room, rings the person and returns the in-app call path. */
  createCall: (title: string, userId: string) => Promise<string>;
  openWindow: (url: string) => void;
  dispatch: (name: string, detail: unknown) => void;
}

const APP_OF_ACTION: Record<string, string> = {
  chat: "/chat",
  email: "/mail",
  schedule: "/calendar",
};

/**
 * Runs an action. When the target app is already open the page is not
 * reloaded: an `adrient:person-action` event carries the request instead, and
 * the app listens for it. Otherwise the router navigates to the app URL, which
 * reads the same query parameters on mount.
 */
export async function runAction(
  action: PersonAction,
  p: PersonTarget,
  deps: ActionDeps,
): Promise<void> {
  if (action === "call") {
    // Same tab, like Teams: the call keeps running in the call bar if the
    // user goes back to another app.
    const path = await deps.createCall(`Call with ${p.name || p.email || "colleague"}`, p.id ?? "");
    deps.navigate(path);
    return;
  }
  const href = actionHref(action, p);
  if (!href) return;
  const app = APP_OF_ACTION[action];
  if (deps.pathname === app || deps.pathname.startsWith(app + "/")) {
    deps.dispatch("adrient:person-action", { action, id: p.id, email: p.email });
    return;
  }
  deps.navigate(href);
}
