import type { MailMessage } from "./types";

/** Recipients for Reply all: the sender plus everyone on To/Cc except the user (deduplicated). */
export function replyAllRecipients(m: MailMessage, myEmail: string): { to: string[]; cc: string[] } {
  const me = myEmail.toLowerCase();
  const seen = new Set<string>([me]);
  const take = (list: string[]) =>
    list.filter((a) => {
      const k = a.trim().toLowerCase();
      if (!k || seen.has(k)) return false;
      seen.add(k);
      return true;
    });
  // Replying to your own message goes to the original recipients.
  const to = m.from_addr.toLowerCase() === me ? take(m.to_addrs ?? []) : take([m.from_addr, ...(m.to_addrs ?? [])]);
  const cc = take(m.cc_addrs ?? []);
  return { to, cc };
}
