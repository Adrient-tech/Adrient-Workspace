import { applyMailLabel, getMessage, getThread, listLabelsWithEntities, listThreads, modifyMessage, removeMailLabel } from "../api";
import { deleteTask } from "../../tasks/api";
import { quickTask } from "../../tasks/extrasApi";
import type { MailMsg, MailAdapter } from "./tools";

export interface MailAdapterHooks {
  openThreadId: () => string | null;
  openCompose: (init: { to: string; cc?: string; subject: string; body: string }) => () => void;
  /** Called after a change so the page reloads. */
  changed: () => void;
}

/** The real adapter: the user's own session against /api/v1/mail. */
export function mailAdapter(h: MailAdapterHooks): MailAdapter {
  const touch = async <T,>(p: Promise<T>) => {
    const r = await p;
    h.changed();
    return r;
  };
  return {
    findThreads: async ({ query, folder, label }) => {
      const r = folder === "starred" ? await listThreads("", { query, label, starred: true }) : await listThreads(label ? "" : folder, { query, label });
      return r.threads ?? [];
    },
    readThread: async (id) => {
      try {
        const msgs = await getThread(id);
        if (msgs.length) return msgs as MailMsg[];
      } catch {
        /* fall through to single message */
      }
      try {
        return [await getMessage(id)] as MailMsg[];
      } catch {
        return [];
      }
    },
    openThreadId: h.openThreadId,
    moveMessage: (m, folder) => touch(modifyMessage(m.id, { is_read: m.is_read, starred: m.starred, folder }).then(() => undefined)),
    setRead: (m, read) => touch(modifyMessage(m.id, { is_read: read, starred: m.starred }).then(() => undefined)),
    listLabels: async () => (await listLabelsWithEntities()).label_objects ?? [],
    applyLabel: (mid, lid) => touch(applyMailLabel(mid, lid)),
    removeLabel: (mid, lid) => touch(removeMailLabel(mid, lid)),
    openCompose: h.openCompose,
    createTask: async (t) => {
      const r = await quickTask({ title: t.title, notes: t.notes, link: t.link, due_at: t.due_at });
      return { id: r.id, list_id: r.list_id, title: r.title };
    },
    deleteTask: (t) => deleteTask(t.list_id, t.id),
  };
}
