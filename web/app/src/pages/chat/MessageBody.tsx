/**
 * MessageBody — renders a chat message's body (Markdown) and its attachments.
 *
 * Images and GIFs render inline (capped at 320px wide, responsive).
 * Other files render as a download chip.
 * Body text is rendered with react-markdown + remark-gfm and sanitized with
 * rehype-sanitize (default schema).
 */

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";
import { Box, Chip, Typography } from "@mui/joy";
import AttachFileIcon from "@mui/icons-material/AttachFile";
import type { ChatAttachment } from "./types";
import { AttachmentCard, isCardAttachment, LinkCards } from "./LinkCards";

interface MessageBodyProps {
  body: string;
  attachments?: ChatAttachment[];
  /** Org member names/emails; "@Name" occurrences render as highlighted chips. */
  mentionNames?: string[];
  /** Name (lower-case) of the signed-in user, to emphasise mentions of me. */
  meName?: string;
}

/** Wraps @mentions in a markdown link (#mention) so they can be styled. */
export function highlightMentions(body: string, names: string[]): string {
  const uniq = Array.from(
    new Set(
      ["channel", "here", "everyone", ...names]
        .map((n) => n.trim())
        .filter(Boolean),
    ),
  ).sort((a, b) => b.length - a.length);
  if (uniq.length === 0 || !body.includes("@")) return body;
  const esc = uniq.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const re = new RegExp(`(^|[\\s(])@(${esc.join("|")})(?![\\w])`, "gi");
  return body.replace(re, (_m, pre: string, name: string) => `${pre}[@${name}](#mention)`);
}

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function isImageMime(mime: string): boolean {
  return mime.startsWith("image/");
}

export function MessageBody({
  body,
  attachments,
  mentionNames,
  meName,
}: MessageBodyProps) {
  const raw = (body ?? "").trim();
  const trimmed = mentionNames ? highlightMentions(raw, mentionNames) : raw;

  return (
    <Box>
      {/* Markdown body */}
      {trimmed && trimmed !== " " && (
        <Box
          sx={{
            "& p": { m: 0, lineHeight: 1.6 },
            "& p + p": { mt: 0.75 },
            "& strong": { fontWeight: 700 },
            "& em": { fontStyle: "italic" },
            "& del": { textDecoration: "line-through" },
            "& code": {
              fontFamily: "monospace",
              fontSize: "0.8em",
              bgcolor: "background.level2",
              px: 0.5,
              py: 0.125,
              borderRadius: "xs",
            },
            "& pre": {
              bgcolor: "background.level2",
              p: 1,
              borderRadius: "sm",
              overflowX: "auto",
              fontSize: "0.8em",
              "& code": { bgcolor: "transparent", p: 0 },
            },
            "& ul, & ol": { pl: 2.5, my: 0.5 },
            "& li": { lineHeight: 1.6 },
            "& a": { color: "primary.500", textDecoration: "underline" },
            "& img": {
              maxWidth: "min(320px, 100%)",
              borderRadius: "sm",
              display: "block",
              mt: 0.5,
            },
            // Tables (remark-gfm)
            "& table": {
              borderCollapse: "collapse",
              width: "100%",
              fontSize: "0.85em",
            },
            "& th, & td": {
              border: "1px solid",
              borderColor: "divider",
              px: 1,
              py: 0.5,
              textAlign: "left",
            },
            "& blockquote": {
              borderLeft: "3px solid",
              borderColor: "primary.300",
              pl: 1.5,
              ml: 0,
              opacity: 0.8,
            },
          }}
        >
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeSanitize]}
            components={{
              a: ({ href, children, ...rest }) =>
                href === "#mention" ? (
                  <Box
                    component="span"
                    sx={{
                      bgcolor:
                        String(children).toLowerCase() ===
                          `@${(meName ?? "").toLowerCase()}` ||
                        /^@(channel|here|everyone)$/i.test(String(children))
                          ? "warning.softBg"
                          : "primary.softBg",
                      color: "primary.700",
                      borderRadius: "sm",
                      px: 0.5,
                      fontWeight: 600,
                    }}
                  >
                    {children}
                  </Box>
                ) : (
                  <a
                    href={href}
                    {...rest}
                    target={href?.startsWith("/") ? undefined : "_blank"}
                    rel="noopener noreferrer"
                  >
                    {children}
                  </a>
                ),
            }}
          >
            {trimmed}
          </ReactMarkdown>
        </Box>
      )}

      {/* Link cards: meetings, files and workspace objects */}
      {raw && <LinkCards body={raw} />}

      {/* Attachments */}
      {attachments && attachments.length > 0 && (
        <Box
          sx={{
            display: "flex",
            flexWrap: "wrap",
            gap: 1,
            mt: trimmed && trimmed !== " " ? 0.75 : 0,
          }}
        >
          {attachments.map((att) =>
            isCardAttachment(att) ? (
              <Box key={att.id} sx={{ flexBasis: "100%", mt: -0.75 }}>
                <AttachmentCard att={att} size={fmtBytes(att.size)} />
              </Box>
            ) : isImageMime(att.mime_type) ? (
              <Box
                key={att.id}
                component="a"
                href={att.url}
                target="_blank"
                rel="noopener noreferrer"
                sx={{ display: "inline-block", lineHeight: 0 }}
              >
                <img
                  src={att.url}
                  alt={att.name}
                  loading="lazy"
                  style={{
                    maxWidth: "min(320px, 100%)",
                    maxHeight: 240,
                    objectFit: "contain",
                    borderRadius: 6,
                    display: "block",
                  }}
                />
              </Box>
            ) : (
              <Chip
                key={att.id}
                size="sm"
                variant="outlined"
                startDecorator={<AttachFileIcon sx={{ fontSize: 14 }} />}
                component="a"
                href={att.url}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  cursor: "pointer",
                  textDecoration: "none",
                  maxWidth: 240,
                }}
              >
                <Typography
                  level="body-xs"
                  sx={{
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {att.name}
                </Typography>
                <Typography
                  level="body-xs"
                  sx={{ opacity: 0.6, ml: 0.5, flexShrink: 0 }}
                >
                  {fmtBytes(att.size)}
                </Typography>
              </Chip>
            ),
          )}
        </Box>
      )}
    </Box>
  );
}
