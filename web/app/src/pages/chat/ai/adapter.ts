import { deleteTask } from "../../tasks/api";
import { quickTask } from "../../tasks/extrasApi";
import { teams } from "../teamsApi";
import type { TChannel } from "../types";
import type { ChatAdapter } from "./tools";

/** The real adapter: the user's own session against /api/v1/chat/x. */
export function chatAdapter(h: { channels: () => TChannel[]; currentChannelId: () => string | null; changed: () => void }): ChatAdapter {
  return {
    channels: async () => h.channels().filter((c) => c.is_member && !c.archived).map((c) => ({ id: c.id, name: c.name, kind: c.kind })),
    currentChannelId: h.currentChannelId,
    messages: async (id, limit) => ((await teams.messages(id)).messages ?? []).slice(-limit),
    thread: (id) => teams.thread(id),
    post: async (ch, body, parent) => {
      const r = await teams.post(ch, body, { parent_id: parent });
      h.changed();
      return r;
    },
    remove: async (id) => {
      await teams.remove(id);
      h.changed();
    },
    createTask: async (t) => {
      const r = await quickTask({ title: t.title, notes: t.notes, link: t.link, due_at: t.due_at });
      return { id: r.id, list_id: r.list_id, title: r.title };
    },
    deleteTask: (t) => deleteTask(t.list_id, t.id),
  };
}
