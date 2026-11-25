import { createMeeting, meetingLink } from "../meet/api";
import { meetPath } from "../../lib/calls";
import { teams } from "./teamsApi";
import type { TChannel } from "./types";

/**
 * Starts an instant meeting. In a conversation it posts a meeting card (the
 * link renders as a "Join meeting" card) and returns the path that opens the
 * call docked in that conversation; without one it returns a plain call path.
 */
export async function meetNow(channel: TChannel | undefined, label: string): Promise<string> {
  const room = await createMeeting(channel ? `Meeting: ${label}` : "Meet now");
  const code = room.code ?? "";
  if (!code) throw new Error("meeting has no code");
  if (channel && channel.is_member && !channel.archived) {
    await teams.post(channel.id, `📞 **Meeting started**\n\n${meetingLink(code)}`);
  }
  return meetPath(code, { returnTo: channel ? `/chat/c/${channel.id}` : "/chat/meetings" });
}
