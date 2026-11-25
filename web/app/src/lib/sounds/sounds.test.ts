import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SoundManager, type AudioLike, type SoundEvent } from "./manager";
import { DEFAULT_PREFS, inQuietHours, isDnd, sanitizePrefs } from "./prefs";
import { CATEGORY_OF, SOUND_IDS, SOUND_SETS, soundForNotification, soundUrl } from "./catalog";

class FakeAudio implements AudioLike {
  src = "";
  volume = 1;
  loop = false;
  currentTime = 0;
  preload = "";
  plays = 0;
  pauses = 0;
  reject = false;
  async play() {
    if (this.reject) throw new DOMException("blocked", "NotAllowedError");
    this.plays++;
  }
  pause() {
    this.pauses++;
  }
  addEventListener() {}
}

function make(over: Partial<{ now: Date; quiet: boolean }> = {}) {
  const made: FakeAudio[] = [];
  const m = new SoundManager({
    createAudio: (url) => {
      const a = new FakeAudio();
      a.src = url;
      made.push(a);
      return a;
    },
    now: () => over.now ?? new Date("2026-03-02T12:00:00"),
    systemQuiet: () => over.quiet ?? false,
    maxLoopMs: 1000,
  });
  const events: SoundEvent[] = [];
  m.onEvent((e) => events.push(e));
  return { m, made, events };
}

describe("catalog", () => {
  it("has a category and a file for every sound in every set", () => {
    for (const id of SOUND_IDS) expect(CATEGORY_OF[id]).toBeTruthy();
    expect(soundUrl("soft", "mail")).toBe("/sounds/soft/mail.mp3");
    expect(SOUND_SETS).toEqual(["classic", "soft", "bright"]);
  });
  it("maps notification kinds to tones", () => {
    expect(soundForNotification("call")).toBeNull();
    expect(soundForNotification("meet_ended")).toBe("meeting_ended");
    expect(soundForNotification("calendar_reminder")).toBe("calendar");
    expect(soundForNotification("task_assigned")).toBe("task");
    expect(soundForNotification("slides_mention")).toBe("mention");
    expect(soundForNotification("expense_approval")).toBe("approval");
    expect(soundForNotification("mail")).toBe("mail");
    expect(soundForNotification("something_else")).toBe("message");
  });
});

describe("prefs", () => {
  it("sanitises junk", () => {
    const p = sanitizePrefs({ volume: 7, set: "nope", enabled: { chat: false, bogus: false }, dnd: { from: "25:00", on: true } });
    expect(p.volume).toBe(1);
    expect(p.set).toBe("classic");
    expect(p.enabled.chat).toBe(false);
    expect(p.enabled.mail).toBe(true);
    expect(p.dnd.from).toBe("22:00");
    expect(p.dnd.on).toBe(true);
    expect(sanitizePrefs(null)).toEqual(sanitizePrefs(undefined));
  });
  it("handles quiet hours that wrap midnight", () => {
    const dnd = { on: false, scheduled: true, from: "22:00", to: "07:00" };
    expect(inQuietHours(dnd, new Date("2026-03-02T23:30:00"))).toBe(true);
    expect(inQuietHours(dnd, new Date("2026-03-02T06:59:00"))).toBe(true);
    expect(inQuietHours(dnd, new Date("2026-03-02T07:00:00"))).toBe(false);
    expect(inQuietHours(dnd, new Date("2026-03-02T12:00:00"))).toBe(false);
    expect(isDnd({ ...dnd, scheduled: false, on: true }, new Date("2026-03-02T12:00:00"))).toBe(true);
  });
});

describe("SoundManager", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("plays the file of the chosen set, at the master volume", async () => {
    const { m, made, events } = make();
    m.setPrefs({ ...DEFAULT_PREFS, set: "bright", volume: 0.5 });
    expect(await m.play("mail")).toBe(true);
    const a = made.find((x) => x.src === "/sounds/bright/mail.mp3")!;
    expect(a.plays).toBe(1);
    expect(a.volume).toBeCloseTo(0.45);
    expect(events.at(-1)).toMatchObject({ id: "mail", kind: "play" });
  });

  it("skips disabled categories, zero volume and Do Not Disturb", async () => {
    const { m, made, events } = make();
    m.setPrefs({ ...DEFAULT_PREFS, enabled: { ...DEFAULT_PREFS.enabled, chat: false } });
    expect(await m.play("message")).toBe(false);
    expect(events.at(-1)).toMatchObject({ kind: "skipped", reason: "category" });
    m.setPrefs({ ...DEFAULT_PREFS, volume: 0 });
    expect(await m.play("mail")).toBe(false);
    expect(events.at(-1)).toMatchObject({ reason: "volume" });
    m.setPrefs({ ...DEFAULT_PREFS, dnd: { ...DEFAULT_PREFS.dnd, on: true } });
    expect(await m.play("mail")).toBe(false);
    expect(await m.loop("ringtone")).toBe(false); // DND silences the ringtone too
    expect(events.at(-1)).toMatchObject({ reason: "dnd" });
    expect(made.every((a) => a.plays === 0)).toBe(true);
  });

  it("force (test buttons) overrides category and DND but not volume 0", async () => {
    const { m } = make();
    m.setPrefs({ ...DEFAULT_PREFS, dnd: { ...DEFAULT_PREFS.dnd, on: true } });
    expect(await m.play("mail", { force: true })).toBe(true);
    m.setPrefs({ ...DEFAULT_PREFS, volume: 0 });
    expect(await m.play("mail", { force: true })).toBe(false);
  });

  it("stays quiet on reduced motion except for calls", async () => {
    const { m } = make({ quiet: true });
    expect(await m.play("message")).toBe(false);
    expect(await m.loop("ringtone")).toBe(true);
    m.setPrefs({ ...DEFAULT_PREFS, followSystem: false });
    expect(await m.play("message")).toBe(true);
  });

  it("loops, stops, and stops by itself after the safety timeout", async () => {
    const { m, made, events } = make();
    expect(await m.loop("ringtone")).toBe(true);
    const a = made.find((x) => x.src.endsWith("/ringtone.mp3"))!;
    expect(a.loop).toBe(true);
    expect(m.isLooping("ringtone")).toBe(true);
    await m.loop("ringtone"); // second request does not restart it
    expect(a.plays).toBe(1);
    m.stop("ringtone");
    expect(a.pauses).toBe(1);
    expect(m.isLooping("ringtone")).toBe(false);
    await m.loop("ringback");
    vi.advanceTimersByTime(1001);
    expect(m.isLooping("ringback")).toBe(false);
    expect(events.filter((e) => e.kind === "stop").length).toBe(2);
  });

  it("silences a running ringtone when Do Not Disturb is switched on", async () => {
    const { m } = make();
    await m.loop("ringtone");
    m.setPrefs({ ...DEFAULT_PREFS, dnd: { ...DEFAULT_PREFS.dnd, on: true } });
    expect(m.isLooping("ringtone")).toBe(false);
  });

  it("keeps a refused loop pending until the first gesture unlocks audio", async () => {
    const { m, made, events } = make();
    m.preload();
    const ring = made.find((x) => x.src.endsWith("/ringtone.mp3"))!;
    ring.reject = true;
    const handlers: Record<string, () => void> = {};
    const target = {
      addEventListener: (t: string, f: () => void) => (handlers[t] = f),
      removeEventListener: () => {},
    };
    // unlockOnGesture() is called inside loop() with the real window; stub window
    vi.stubGlobal("window", Object.assign(target, { dispatchEvent: () => true }));
    expect(await m.loop("ringtone")).toBe(false);
    expect(events.some((e) => e.kind === "blocked")).toBe(true);
    ring.reject = false;
    handlers["pointerdown"]();
    await vi.advanceTimersByTimeAsync(0);
    expect(m.isLooping("ringtone")).toBe(true);
    vi.unstubAllGlobals();
  });

  it("reloads the files when the set changes and stops loops", async () => {
    const { m, made } = make();
    await m.loop("ringtone");
    m.setPrefs({ ...DEFAULT_PREFS, set: "soft" });
    expect(m.isLooping("ringtone")).toBe(false);
    expect(made.some((a) => a.src === "/sounds/soft/ringtone.mp3")).toBe(true);
  });

  it("preload creates one element per sound", () => {
    const { m, made } = make();
    m.preload();
    expect(made.length).toBe(SOUND_IDS.length);
  });
});
