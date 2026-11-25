import { beforeEach, describe, expect, it } from "vitest";
import { applyMeetEvent, fmtMeetDuration, infoFromRoom, resetMeetStatus } from "./meetStatus";

describe("meetStatus", () => {
  beforeEach(() => resetMeetStatus());
  it("formats durations", () => {
    expect(fmtMeetDuration(45)).toBe("45s");
    expect(fmtMeetDuration(723)).toBe("12m 03s");
    expect(fmtMeetDuration(3900)).toBe("1h 05m");
  });
  it("reads the ended fields of a resolved room", () => {
    const i = infoFromRoom({ id: "1", org_id: "o", owner_id: "u", name: "Sync", created_at: "", ended: true, duration_s: 60, participant_count: 2, ended_at: "t" });
    expect(i).toEqual({ name: "Sync", ended: true, durationS: 60, participants: 2, endedAt: "t" });
    expect(infoFromRoom({ id: "1", org_id: "o", owner_id: "u", name: "", created_at: "" }).ended).toBe(false);
  });
  it("applies live events without throwing on junk", () => {
    applyMeetEvent({ type: "meet_ended" });
    applyMeetEvent({ type: "meet_ended", code: "abc-defg-hij", duration_s: 90, participant_count: 3, ended_at: "x" });
    applyMeetEvent({ type: "meet_restarted", code: "abc-defg-hij" });
  });
});
