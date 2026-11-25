import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Alert, Box, Button, CircularProgress, Input, Sheet, Textarea, Typography } from "@mui/joy";
import * as api from "./helpApi";
import { parseArticle, splitLinks } from "./helpLogic";

const ACCENT = "#126C5A";

function Text({ text }: { text: string }) {
  return (
    <>
      {splitLinks(text).map((p, i) =>
        p.href ? (
          <a key={i} href={p.href} target="_blank" rel="noreferrer noopener">{p.text}</a>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </>
  );
}

/** Public help centre at /help/:token (and /help/:token/:slug). No sign-in. */
export function HelpCenterPage() {
  const { token = "", slug } = useParams();
  const [data, setData] = useState<{ name: string; articles: api.Article[] } | null>(null);
  const [missing, setMissing] = useState(false);
  const [q, setQ] = useState("");
  useEffect(() => {
    api.publicHelp(token).then(setData).catch(() => setMissing(true));
  }, [token]);
  const list = useMemo(() => {
    const s = q.trim().toLowerCase();
    return (data?.articles ?? []).filter((a) => !s || a.title.toLowerCase().includes(s) || a.body.toLowerCase().includes(s));
  }, [data, q]);
  if (missing) return <Box sx={{ p: 6, textAlign: "center" }}><Typography level="h3" data-testid="help-missing">This help centre is not available</Typography></Box>;
  if (!data) return <Box sx={{ p: 6, textAlign: "center" }}><CircularProgress /></Box>;
  const article = slug ? data.articles.find((a) => a.slug === slug) : undefined;
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", py: 4, px: 1.5 }}>
      <Box sx={{ maxWidth: 760, mx: "auto" }}>
        <Typography level="h2" sx={{ color: ACCENT }} data-testid="help-title">Help centre: {data.name}</Typography>
        {article ? (
          <Sheet variant="outlined" sx={{ p: 3, mt: 2, borderRadius: "lg" }} data-testid="help-article">
            <Button component="a" href={`/help/${token}`} size="sm" variant="plain" sx={{ mb: 1 }}>All articles</Button>
            <Typography level="h3">{article.title}</Typography>
            {parseArticle(article.body).map((b, i) =>
              b.kind === "h" ? (
                <Typography key={i} level="title-lg" sx={{ mt: 2 }}>{b.text}</Typography>
              ) : b.kind === "ul" ? (
                <Box key={i} component="ul" sx={{ pl: 3, my: 1 }}>
                  {b.items.map((it, j) => <li key={j}><Text text={it} /></li>)}
                </Box>
              ) : (
                <Typography key={i} sx={{ my: 1 }}><Text text={b.text} /></Typography>
              ),
            )}
          </Sheet>
        ) : slug ? (
          <Typography sx={{ mt: 2 }}>That article was not found.</Typography>
        ) : (
          <>
            <Input placeholder="Search articles" value={q} onChange={(e) => setQ(e.target.value)} sx={{ my: 2 }} slotProps={{ input: { "data-testid": "help-search" } }} />
            {list.length === 0 && <Typography>No articles found.</Typography>}
            {list.map((a) => (
              <Sheet key={a.id} variant="outlined" component="a" href={`/help/${token}/${a.slug}`} sx={{ display: "block", p: 2, mb: 1, borderRadius: "md", textDecoration: "none", color: "inherit", "&:hover": { bgcolor: "neutral.softBg" } }} data-testid="help-item">
                <Typography level="title-md">{a.title}</Typography>
              </Sheet>
            ))}
          </>
        )}
      </Box>
    </Box>
  );
}

/** Satisfaction survey at /csat/:token. */
export function CsatPage() {
  const { token = "" } = useParams();
  const [info, setInfo] = useState<api.Csat | null>(null);
  const [missing, setMissing] = useState(false);
  const [rating, setRating] = useState(0);
  const [comment, setComment] = useState("");
  const [done, setDone] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => {
    api.publicCsat(token).then(setInfo).catch(() => setMissing(true));
  }, [token]);
  if (missing) return <Box sx={{ p: 6, textAlign: "center" }}><Typography level="h3">Survey not found</Typography></Box>;
  if (!info) return <Box sx={{ p: 6, textAlign: "center" }}><CircularProgress /></Box>;
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.level1", display: "grid", placeItems: "center", p: 2 }}>
      <Sheet variant="outlined" sx={{ p: 3, borderRadius: "lg", maxWidth: 480, width: "100%" }} data-testid="csat-page">
        <Typography level="title-lg" sx={{ color: ACCENT }}>How did we do?</Typography>
        <Typography level="body-sm" sx={{ mb: 2 }}>{info.ref}: {info.title}</Typography>
        {info.answered || done ? (
          <Alert color="success" data-testid="csat-thanks">Thank you for your feedback.</Alert>
        ) : (
          <>
            <Box sx={{ display: "flex", gap: 1, mb: 2 }} role="radiogroup" aria-label="Rating">
              {[1, 2, 3, 4, 5].map((n) => (
                <Button key={n} variant={rating === n ? "solid" : "outlined"} color={rating === n ? "primary" : "neutral"} onClick={() => setRating(n)} aria-label={`${n} out of 5`} role="radio" aria-checked={rating === n} data-testid={`rate-${n}`} sx={rating === n ? { bgcolor: ACCENT } : {}}>
                  {n}
                </Button>
              ))}
            </Box>
            <Typography level="body-xs" sx={{ mb: 1 }}>1 is poor, 5 is excellent.</Typography>
            <Textarea minRows={3} placeholder="Anything we should know? (optional)" value={comment} onChange={(e) => setComment(e.target.value)} slotProps={{ textarea: { maxLength: 2000 } }} />
            {err && <Alert color="danger" sx={{ mt: 1 }}>{err}</Alert>}
            <Button sx={{ mt: 2, bgcolor: ACCENT }} disabled={!rating} data-testid="csat-send" onClick={() => api.answerCsat(token, rating, comment).then(() => setDone(true)).catch((e: Error) => setErr(e.message))}>
              Send
            </Button>
          </>
        )}
      </Sheet>
    </Box>
  );
}
