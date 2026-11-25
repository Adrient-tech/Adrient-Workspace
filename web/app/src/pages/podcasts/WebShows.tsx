/** "Shows from the web": subscribe to outside audio feeds by address (server side, address-checked fetch). */
import { useCallback, useEffect, useState } from "react";
import { Box } from "@mui/joy";
import RssFeedRounded from "@mui/icons-material/RssFeedRounded";
import { getFeed, listSubscriptions, subscribe, unsubscribe, type Feed, type Subscription } from "./api";
import { Btn, C, Cover, Empty, ErrorNote, FONT_DISPLAY, Skeleton, TextInput, errMsg, ago, useToast } from "./ui";

export function WebShowsPage() {
  const toast = useToast();
  const [subs, setSubs] = useState<Subscription[] | null>(null);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [open, setOpen] = useState<{ sub: Subscription; feed?: Feed; err?: string } | null>(null);
  const [nowPlaying, setNowPlaying] = useState<{ title: string; src: string } | null>(null);
  const load = useCallback(() => { listSubscriptions().then(setSubs).catch((e) => setError(errMsg(e))); }, []);
  useEffect(load, [load]);

  const add = async () => {
    setBusy(true);
    try {
      const feed = await getFeed(url.trim());
      await subscribe({ feed_url: url.trim(), title: feed.title, author: feed.author, artwork_url: feed.image });
      setUrl("");
      toast.show(`Subscribed to ${feed.title || "the show"}`);
      load();
    } catch (e) { toast.show(errMsg(e).includes("400") ? "That address is not allowed or not a web address." : "Could not read a show at that address.", "error"); } finally { setBusy(false); }
  };
  const view = async (sub: Subscription) => {
    setOpen({ sub });
    try { setOpen({ sub, feed: await getFeed(sub.feed_url) }); } catch { setOpen({ sub, err: "Could not load this show right now." }); }
  };
  return (
    <Box>
      <Box component="h1" sx={{ m: 0, mb: 0.5, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: { xs: 26, md: 32 }, letterSpacing: "-0.03em" }}>Shows from the web</Box>
      <Box sx={{ color: C.muted, fontSize: 14.5, mb: 2.5 }}>Follow audio shows published outside the company by pasting their feed address.</Box>
      <Box sx={{ display: "flex", gap: 1, maxWidth: 640, mb: 3 }}>
        <TextInput value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.org/feed.xml" aria-label="Feed address" testId="feed-url" onKeyDown={(e) => e.key === "Enter" && url.trim() && void add()} />
        <Btn variant="primary" onClick={() => void add()} disabled={busy || !url.trim()} testId="feed-add">Subscribe</Btn>
      </Box>
      {error && <ErrorNote message={error} onRetry={load} />}
      {!subs && !error && <Skeleton h={90} r={16} />}
      {subs && subs.length === 0 && <Empty icon={<RssFeedRounded fontSize="inherit" />} title="No web shows yet" hint="Paste a feed address above to subscribe. Nothing is added automatically." />}
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))", gap: 2 }}>
        {subs?.map((s) => (
          <Box key={s.id} sx={{ display: "flex", gap: 1.6, background: C.surface, border: `1px solid ${C.line}`, borderRadius: "18px", p: 1.5, alignItems: "center" }}>
            <Cover title={s.title || s.feed_url} url={s.artwork_url || undefined} style="200,240,135" size={72} radius={12} />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ fontWeight: 700, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{s.title || s.feed_url}</Box>
              <Box sx={{ fontSize: 12.5, color: C.muted }}>{s.author}</Box>
              <Box sx={{ display: "flex", gap: 1, mt: 0.8 }}>
                <Btn size="sm" variant="soft" onClick={() => void view(s)}>Episodes</Btn>
                <Btn size="sm" variant="ghost" onClick={async () => { try { await unsubscribe(s.id); load(); } catch (e) { toast.show(errMsg(e), "error"); } }}>Unsubscribe</Btn>
              </Box>
            </Box>
          </Box>
        ))}
      </Box>
      {open && (
        <Box sx={{ mt: 3, background: C.surface, border: `1px solid ${C.line}`, borderRadius: "18px", p: 2 }}>
          <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, mb: 1 }}>{open.sub.title}</Box>
          {!open.feed && !open.err && <Skeleton h={50} />}
          {open.err && <ErrorNote message={open.err} />}
          {open.feed?.episodes.slice(0, 20).map((e) => (
            <Box key={e.guid || e.audio_url} sx={{ display: "flex", gap: 1.5, py: 1, borderTop: `1px solid ${C.line}`, alignItems: "center" }}>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Box sx={{ fontWeight: 650, fontSize: 14.5 }}>{e.title}</Box>
                <Box sx={{ fontSize: 12.5, color: C.muted }}>{e.published ? ago(e.published) : ""} {e.duration ? `· ${e.duration}` : ""}</Box>
              </Box>
              <Btn size="sm" variant="soft" disabled={!e.audio_url} onClick={() => setNowPlaying({ title: e.title, src: e.audio_url })}>Play</Btn>
            </Box>
          ))}
        </Box>
      )}
      {nowPlaying && (
        <Box sx={{ position: "fixed", left: 16, right: 16, bottom: 16, zIndex: 1250, background: C.surface, borderRadius: "16px", boxShadow: "0 8px 30px rgba(0,0,0,.2)", p: 1.5, display: "flex", gap: 1.5, alignItems: "center", flexWrap: "wrap" }}>
          <Box sx={{ fontWeight: 700, fontSize: 14, flex: "1 1 200px" }}>{nowPlaying.title}</Box>
          <audio src={nowPlaying.src} controls autoPlay style={{ flex: "2 1 300px", maxWidth: "100%" }} />
          <Btn size="sm" variant="ghost" onClick={() => setNowPlaying(null)}>Close</Btn>
        </Box>
      )}
    </Box>
  );
}
