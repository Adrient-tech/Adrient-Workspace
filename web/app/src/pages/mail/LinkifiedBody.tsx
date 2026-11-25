import { Fragment, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Link as JoyLink } from "@mui/joy";
import VideocamIcon from "@mui/icons-material/Videocam";
import { openLink, withReturn } from "../../lib/openWith";

const TOKEN = /(https?:\/\/[^\s<>"')\]]+|\b[a-z]{3}-[a-z]{4}-[a-z]{3}\b)/gi;

/** Splits plain text into text / link parts (exported for tests). */
export function splitLinks(text: string): Array<{ text: string; link: boolean }> {
  const out: Array<{ text: string; link: boolean }> = [];
  let last = 0;
  for (const m of text.matchAll(TOKEN)) {
    const i = m.index ?? 0;
    let tok = m[0];
    // Trailing sentence punctuation is not part of the URL.
    const trail = /[.,;:!?]+$/.exec(tok);
    if (trail && tok.startsWith("http")) tok = tok.slice(0, -trail[0].length);
    if (i > last) out.push({ text: text.slice(last, i), link: false });
    out.push({ text: tok, link: true });
    last = i + tok.length;
  }
  if (last < text.length) out.push({ text: text.slice(last), link: false });
  return out;
}

/**
 * Mail body text with clickable links. Workspace links open in-app (the global
 * link handler intercepts plain anchors); meeting links and codes also get a
 * "Join meeting" button that returns to Mail.
 */
export function LinkifiedBody({ text }: { text: string }): ReactNode {
  const navigate = useNavigate();
  return (
    <>
      {splitLinks(text).map((p, i) => {
        if (!p.link) return <Fragment key={i}>{p.text}</Fragment>;
        const t = openLink(p.text);
        if (!t) return <Fragment key={i}>{p.text}</Fragment>;
        const isMeet = t.app === "meet" && !t.external;
        return (
          <Fragment key={i}>
            <JoyLink
              href={t.href}
              target={t.external ? "_blank" : undefined}
              rel={t.external ? "noopener noreferrer" : undefined}
            >
              {p.text}
            </JoyLink>
            {isMeet && (
              <Button
                size="sm"
                variant="soft"
                startDecorator={<VideocamIcon />}
                sx={{ ml: 1, verticalAlign: "middle" }}
                data-testid="mail-join-meeting"
                onClick={() => navigate(withReturn(t.href, "/mail"))}
              >
                Join meeting
              </Button>
            )}
          </Fragment>
        );
      })}
    </>
  );
}
