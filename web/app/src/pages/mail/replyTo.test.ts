import { describe, expect, it } from "vitest";
import { replyAllRecipients } from "./replyTo";
import type { MailMessage } from "./types";

const base = { from_addr: "a@x.com", to_addrs: ["me@x.com", "b@x.com"], cc_addrs: ["c@x.com", "A@x.com"] } as MailMessage;

describe("replyAllRecipients", () => {
  it("removes me and duplicates", () => {
    expect(replyAllRecipients(base, "me@x.com")).toEqual({ to: ["a@x.com", "b@x.com"], cc: ["c@x.com"] });
  });
  it("answers the original recipients when I sent it", () => {
    expect(replyAllRecipients({ ...base, from_addr: "ME@x.com" }, "me@x.com").to).toEqual(["b@x.com"]);
  });
});
