// Browser check for "meeting ended" + sounds. Two browser contexts (A = host, B = member).
//
//   ADRIENT_URL=http://workspace.localtest.me:8141 TOKEN_A=<session token> TOKEN_B=<session token> \
//   PLAYWRIGHT_CORE=/path/to/playwright-core CHROMIUM_PATH=/path/to/chrome SHOTS=/tmp/shots \
//   USER_B_ID=<user id of B> node scripts/qa-meet-ended.mjs
//
// The tokens are rows in the scratch database's sessions table (never the shared one).
import { mkdirSync } from "node:fs";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const BASE = (process.env.ADRIENT_URL || "http://workspace.localtest.me:8141").replace(/\/$/, "");
const SHOTS = process.env.SHOTS || "./shots";
mkdirSync(SHOTS, { recursive: true });
const req = createRequire(pathToFileURL(process.env.PLAYWRIGHT_CORE + "/").href);
const { chromium } = req(process.env.PLAYWRIGHT_CORE);

const results = [];
const check = (name, ok, extra = "") => {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? "  " + extra : ""}`);
};

const browser = await chromium.launch({
  executablePath: process.env.CHROMIUM_PATH || undefined,
  args: ["--no-sandbox", "--use-fake-device-for-media-stream", "--use-fake-ui-for-media-stream", "--autoplay-policy=no-user-gesture-required", `--unsafely-treat-insecure-origin-as-secure=${BASE}`],
});

async function ctx(token) {
  const c = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: "light", permissions: ["camera", "microphone"] });
  await c.addCookies([{ name: "adrient_session", value: token, domain: "workspace.localtest.me", path: "/", httpOnly: true, sameSite: "Lax" }]);
  await c.addInitScript(() => {
    window.__snd = [];
    window.addEventListener("adrient:sound", (e) => window.__snd.push(e.detail));
    const orig = HTMLMediaElement.prototype.play;
    window.__plays = [];
    HTMLMediaElement.prototype.play = function () {
      window.__plays.push(this.src.replace(location.origin, ""));
      return orig.apply(this, arguments);
    };
  });
  return c;
}
const A = await ctx(process.env.TOKEN_A);
const B = await ctx(process.env.TOKEN_B);
const notFound = [];
for (const c of [A, B]) c.on("response", (r) => r.url().includes("/sounds/") && r.status() >= 400 && notFound.push(r.url()));

const api = (page, method, path, body) =>
  page.evaluate(async ([method, path, body]) => {
    const r = await fetch(path, { method, credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: body ? JSON.stringify(body) : undefined });
    let j = null;
    try { j = await r.json(); } catch {}
    return { status: r.status, json: j };
  }, [method, path, body]);
const snd = (page) => page.evaluate(() => window.__snd);
const plays = (page) => page.evaluate(() => window.__plays);

const a1 = await A.newPage();
const b1 = await B.newPage();
await a1.goto(BASE + "/chat");
await b1.goto(BASE + "/chat");
// start from default sound preferences (a previous run may have left Do Not Disturb on)
for (const p of [a1, b1]) {
  const cur = (await api(p, "GET", "/api/v1/me/preferences")).json;
  const x = JSON.parse(cur.extra || "{}");
  delete x.sounds;
  await api(p, "PATCH", "/api/v1/me/preferences", { extra: JSON.stringify(x) });
}
await a1.reload();
await b1.reload();
await a1.waitForTimeout(1500);

// ---- setup: a public channel, a meeting, the "Meeting started" message ----
const ch = (await api(a1, "POST", "/api/v1/chat/x/channels", { kind: "public", name: "w8e2e" + Date.now().toString(36) })).json;
await api(b1, "POST", `/api/v1/chat/x/channels/${ch.id}/join`);
const room = (await api(a1, "POST", "/api/v1/meet/codes", { name: "Meeting: " + ch.name })).json;
const link = `${BASE}/meet/${room.code}`;
await api(a1, "POST", `/api/v1/chat/x/channels/${ch.id}/messages`, { body: `📞 **Meeting started**\n\n${link}`, parent_id: "", attachment_ids: [], client_nonce: "" });
await a1.goto(`${BASE}/chat/c/${ch.id}`);
await b1.goto(`${BASE}/chat/c/${ch.id}`);
await a1.waitForSelector('[data-testid="meet-card"]', { timeout: 20000 });
await b1.waitForSelector('[data-testid="meet-card"]', { timeout: 20000 });
check("card shows Join meeting before the call (A)", (await a1.locator('[data-testid="meet-card-join"]').count()) === 1);
await b1.screenshot({ path: `${SHOTS}/01-b-card-before.png` });

// ---- both join the call in their own tabs ----
const a2 = await A.newPage();
const b2 = await B.newPage();
for (const p of [a2, b2]) {
  await p.goto(`${BASE}/meet/${room.code}`);
  await p.getByRole("button", { name: /join now/i }).click({ timeout: 30000 });
}
await a2.waitForSelector('button[aria-label="Leave call"]', { timeout: 30000 });
await b2.waitForSelector('button[aria-label="Leave call"]', { timeout: 30000 });
await a2.waitForTimeout(3000);
const aSnd = await plays(a2);
const bSnd = await plays(b2);
check("join chime played in A when B joined", aSnd.some((s) => s.endsWith("/join.mp3")), JSON.stringify(aSnd));
await a2.screenshot({ path: `${SHOTS}/02-a-call.png` });

// mute tick + screen of hang-up button
// fake devices start with the mic on; whichever state it is in, toggle twice.
const micBtn = a2.locator('button[aria-label="Mute microphone"], button[aria-label="Unmute microphone"]');
await micBtn.click();
await a2.waitForTimeout(500);
await micBtn.click();
await a2.waitForTimeout(500);
const pl = await plays(a2);
check("mute tick played", pl.some((s) => s.endsWith("/mute.mp3")), JSON.stringify(pl));
check("unmute tick played", pl.some((s) => s.endsWith("/unmute.mp3")));
const hang = a2.locator('button[aria-label="Leave call"]');
await hang.screenshot({ path: `${SHOTS}/03-hangup-button.png` });
await a2.screenshot({ path: `${SHOTS}/03b-controls.png`, clip: { x: 300, y: 800, width: 840, height: 100 } });

// ---- host ends for everyone ----
await a2.getByRole("button", { name: /participants|people/i }).first().click().catch(() => {});
const endBtn = a2.getByRole("button", { name: /end for everyone/i });
if (await endBtn.count()) await endBtn.first().click();
else await api(a2, "POST", `/api/v1/meet/rooms/${room.id}/control`, { action: "end" });
await a1.waitForSelector('[data-testid="meet-card"][data-state="ended"]', { timeout: 20000 }).catch(() => {});
await b1.waitForSelector('[data-testid="meet-card"][data-state="ended"]', { timeout: 20000 }).catch(() => {});
check("host card (A) turned to Meeting ended without reload", (await a1.locator('[data-testid="meet-card"][data-state="ended"]').count()) === 1);
check("member card (B) turned to Meeting ended without reload", (await b1.locator('[data-testid="meet-card"][data-state="ended"]').count()) === 1);
check("Join button gone on the ended card", (await b1.locator('[data-testid="meet-card-join"]').count()) === 0);
console.log("   ended text:", await b1.locator('[data-testid="meet-card-ended"]').first().innerText().catch(() => "-"));
await a1.screenshot({ path: `${SHOTS}/04-a-card-ended.png` });
await b1.screenshot({ path: `${SHOTS}/05-b-card-ended.png` });
await b2.waitForTimeout(1500);
check("meeting_ended tone played in B's call tab", (await plays(b2)).some((s) => s.endsWith("/meeting_ended.mp3")), JSON.stringify(await plays(b2)));

// ---- notification ----
const notes = (await api(b1, "GET", "/api/v1/notifications?limit=10")).json;
const list = notes?.notifications ?? notes?.items ?? [];
check("B has a 'Meeting ended' notification", JSON.stringify(list).includes("Meeting ended"), JSON.stringify(list).slice(0, 200));
await b1.waitForTimeout(500);
await b1.screenshot({ path: `${SHOTS}/06-b-toast.png` });

// ---- ended screen and restart ----
const b3 = await B.newPage();
await b3.goto(`${BASE}/meet/${room.code}`);
await b3.waitForSelector('[data-testid="meet-ended"]', { timeout: 15000 });
check("/meet/<code> shows 'This meeting has ended' to a member", (await b3.getByText("This meeting has ended").count()) === 1);
check("member has no restart button", (await b3.locator('[data-testid="meet-restart"]').count()) === 0);
await b3.screenshot({ path: `${SHOTS}/07-b-ended-screen.png` });
const a3 = await A.newPage();
await a3.goto(`${BASE}/meet/${room.code}`);
await a3.waitForSelector('[data-testid="meet-restart"]', { timeout: 15000 });
await a3.screenshot({ path: `${SHOTS}/08-a-ended-screen.png` });
await a3.locator('[data-testid="meet-restart"]').click();
await a3.getByRole("button", { name: /join now/i }).waitFor({ timeout: 20000 });
check("host can start a new meeting (green room appears)", true);
await b1.waitForSelector('[data-testid="meet-card"][data-state="open"]', { timeout: 15000 }).catch(() => {});
check("card goes back to Join after restart (live)", (await b1.locator('[data-testid="meet-card-join"]').count()) === 1);
await b1.screenshot({ path: `${SHOTS}/09-b-card-restarted.png` });

// ---- ringing: A rings B ----
await api(a1, "POST", "/api/v1/chat/x/calls/ring", { user_ids: [process.env.USER_B_ID], code: room.code, title: "QA call", video: false });
await b1.waitForSelector('[data-testid="incoming-call"]', { timeout: 15000 });
await b1.waitForTimeout(800);
check("ringtone loop started on incoming call", (await snd(b1)).some((e) => e.id === "ringtone" && e.kind === "loop"), JSON.stringify(await snd(b1)));
await b1.screenshot({ path: `${SHOTS}/10-b-incoming.png` });
await b1.locator('[data-testid="call-decline"]').click();
await b1.waitForTimeout(500);
check("ringtone stopped on decline", (await snd(b1)).some((e) => e.id === "ringtone" && e.kind === "stop"));

// ---- all tone files load ----
const files = await b1.evaluate(async () => {
  const sets = ["classic", "soft", "bright"];
  const ids = ["ringtone", "ringback", "join", "leave", "hand", "mute", "unmute", "share", "rec_start", "rec_stop", "meeting_ended", "message", "dm", "mention", "mail", "calendar", "task", "approval", "alert", "success", "error"];
  const bad = [];
  let n = 0;
  await Promise.all(sets.flatMap((s) => ids.map((id) => new Promise((res) => {
    const a = new Audio();
    a.preload = "auto";
    a.onloadedmetadata = () => { n++; if (!(a.duration > 0.05)) bad.push(`${s}/${id} dur=${a.duration}`); res(); };
    a.onerror = () => { bad.push(`${s}/${id} error`); res(); };
    a.src = `/sounds/${s}/${id}.mp3`;
  }))));
  return { n, bad };
});
check("all 63 tone files load as audio with a duration", files.n === 63 && files.bad.length === 0, JSON.stringify(files));
check("no /sounds/ request returned 4xx/5xx", notFound.length === 0, notFound.join(","));

// ---- Settings > Sounds ----
const s1 = await B.newPage();
await s1.addInitScript(() => localStorage.removeItem("adrient.sounds"));
await s1.goto(`${BASE}/settings/sounds`);
await s1.waitForSelector('[data-testid="sounds-page"]');
await s1.screenshot({ path: `${SHOTS}/11-settings-sounds.png`, fullPage: true });
await s1.locator('[data-testid="sound-test-mail"]').click();
await s1.waitForTimeout(500);
check("test button plays the sound", (await snd(s1)).some((e) => e.id === "mail" && e.kind === "play"), JSON.stringify(await snd(s1)));
await s1.locator('[data-testid="sound-set-bright"]').click();
await s1.waitForTimeout(800);
await s1.locator('[data-testid="sound-test-join"]').click();
await s1.waitForTimeout(500);
check("bright set plays bright files", (await plays(s1)).some((p) => p === "/sounds/bright/join.mp3"), JSON.stringify(await plays(s1)));
await s1.locator('[data-testid="sound-dnd"]').click();
await s1.waitForTimeout(800);
await s1.reload();
await s1.waitForSelector('[data-testid="sounds-page"]');
await s1.waitForTimeout(1000);
check("preferences persist across reload (account)", (await s1.locator('[data-testid="sound-dnd"] input').isChecked()) && (await s1.locator('[data-testid="sound-set-bright"]').getAttribute("aria-checked")) === "true");
await s1.screenshot({ path: `${SHOTS}/12-settings-sounds-dnd.png` });
// restore defaults so the shared test account is left clean
await s1.locator('[data-testid="sound-dnd"]').click();
await s1.locator('[data-testid="sound-set-classic"]').click();
await s1.waitForTimeout(800);

await browser.close();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} passed`);
process.exit(failed.length ? 1 : 0);
