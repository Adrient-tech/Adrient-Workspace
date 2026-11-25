import { permalink } from "./lib";
import { makeTask, titleFrom } from "../../lib/makeTask";
import { makeTicket } from "../../lib/makeTicket";
import { teams } from "./teamsApi";
import type { MessageActions } from "./Message";
import type { TMessage } from "./types";

export interface ActionDeps {
  /** Patch a message in whatever list the view holds. */
  update: (id: string, patch: Partial<TMessage>) => void;
  remove: (id: string) => void;
  openThread?: (m: TMessage) => void;
  toast: (title: string, body?: string) => void;
}

/** Builds the per-message action handlers shared by every message list. */
export function buildActions(d: ActionDeps): MessageActions {
  const guard = async <T,>(p: Promise<T>): Promise<T | undefined> => {
    try {
      return await p;
    } catch (e) {
      d.toast("Action failed", (e as Error).message);
      return undefined;
    }
  };
  return {
    openThread: d.openThread,
    react: async (m, emoji) => {
      const r = await guard(teams.react(m.id, emoji));
      if (r) d.update(m.id, { reaction_details: r.reaction_details });
    },
    edit: async (m, body) => {
      const r = await guard(teams.edit(m.id, body));
      if (r) d.update(m.id, { body: r.body, edited_at: r.edited_at, mention_ids: r.mention_ids });
    },
    remove: async (m) => {
      if (!window.confirm("Delete this message? This can't be undone.")) return;
      if (await guard(teams.remove(m.id))) d.remove(m.id);
    },
    pin: async (m) => {
      const r = await guard(teams.pin(m.id));
      if (r) d.update(m.id, { pinned: r.pinned });
    },
    save: async (m) => {
      const r = await guard(teams.save(m.id));
      if (r) d.update(m.id, { saved: r.saved });
    },
    remind: async (m, at) => {
      if (await guard(teams.remind(m.id, at.toISOString()))) d.toast("Reminder set", `We'll remind you at ${at.toLocaleString()}`);
    },
    makeTask: async (m) => {
      try {
        await makeTask({ title: titleFrom(m.body, "Chat follow-up"), notes: m.body.slice(0, 2000), link: permalink(m.channel_id, m.id) });
        d.toast("Task added", "Saved to your Inbox list in Tasks");
      } catch (e) {
        d.toast("Could not add the task", (e as Error).message);
      }
    },
    makeTicket: async (m) => {
      try {
        const t = await makeTicket({ source: "chat", message_id: m.id });
        d.toast("Ticket created", `${t.ref} was added to Tickets`);
      } catch (e) {
        d.toast("Could not create the ticket", (e as Error).message);
      }
    },
    copyLink: async (m) => {
      try {
        await navigator.clipboard.writeText(permalink(m.channel_id, m.id));
        d.toast("Link copied", "Permalink to this message is on your clipboard");
      } catch {
        d.toast("Copy failed", permalink(m.channel_id, m.id));
      }
    },
  };
}
