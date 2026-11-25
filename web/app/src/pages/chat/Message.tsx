import { PersonCard } from "../../components/PersonCard";
import { useState } from "react";
import {
  Avatar,
  Box,
  Button,
  Chip,
  Dropdown,
  IconButton,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Textarea,
  Tooltip,
  Typography,
  ListDivider,
} from "@mui/joy";
import EmojiPicker from "@emoji-mart/react";
import emojiData from "@emoji-mart/data";
import AddReactionOutlinedIcon from "@mui/icons-material/AddReactionOutlined";
import ModeCommentOutlinedIcon from "@mui/icons-material/ModeCommentOutlined";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import PushPinOutlinedIcon from "@mui/icons-material/PushPinOutlined";
import PushPinIcon from "@mui/icons-material/PushPin";
import BookmarkBorderIcon from "@mui/icons-material/BookmarkBorder";
import BookmarkIcon from "@mui/icons-material/Bookmark";
import { MessageBody } from "./MessageBody";
import { colorFor, fmtClock, initials, presets, type PeopleMap } from "./lib";
import type { TMessage } from "./types";

const QUICK = ["👍", "❤️", "😂", "🎉", "🙏", "👀"];

export interface MessageActions {
  react: (m: TMessage, emoji: string) => void;
  openThread?: (m: TMessage) => void;
  edit: (m: TMessage, body: string) => Promise<void>;
  remove: (m: TMessage) => void;
  pin: (m: TMessage) => void;
  save: (m: TMessage) => void;
  remind: (m: TMessage, at: Date) => void;
  copyLink: (m: TMessage) => void;
  makeTask: (m: TMessage) => void;
  makeTicket?: (m: TMessage) => void;
}

interface Props {
  m: TMessage;
  me: string;
  meName: string;
  people: PeopleMap;
  mentionNames: string[];
  actions: MessageActions;
  compact?: boolean;
  /** Hide thread affordances (inside the thread panel). */
  inThread?: boolean;
  highlight?: boolean;
  /** Channel label shown above the message in cross-channel lists. */
  context?: string;
  onContextClick?: () => void;
}

export function UserAvatar({ id, name, size = 36 }: { id: string; name: string; size?: number }) {
  return (
    <Avatar
      sx={{ bgcolor: colorFor(id), color: "#fff", width: size, height: size, fontSize: size * 0.38, flexShrink: 0, fontWeight: 600 }}
    >
      {initials(name)}
    </Avatar>
  );
}

export function MessageItem({
  m, me, meName, people, mentionNames, actions, compact, inThread, highlight, context, onContextClick,
}: Props) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const mine = m.sender_id === me;
  // Teams-style: my messages sit on the right (no avatar/name); others on the left.
  const right = mine && !context;
  const name = people[m.sender_id]?.name || m.sender_name || "Unknown";

  const saveEdit = async () => {
    const body = draft.trim();
    if (!body || body === m.body) {
      setEditing(false);
      return;
    }
    await actions.edit(m, body);
    setEditing(false);
  };

  return (
    <Box
      data-msg-id={m.id}
      data-testid="chat-message"
      sx={{
        position: "relative",
        display: "flex",
        flexDirection: right ? "row-reverse" : "row",
        gap: 1.25,
        px: 2,
        py: compact ? 0.15 : 0.6,
        mt: compact ? 0 : 0.5,
        bgcolor: highlight ? "warning.softBg" : "transparent",
        transition: "background-color 1.5s",
        "&:hover": { bgcolor: highlight ? "warning.softBg" : "transparent" },
        "&:hover .msg-actions": { opacity: 1, pointerEvents: "auto" },
        "&:hover .compact-time": { opacity: 0.6 },
      }}
    >
      <Box sx={{ width: right ? 0 : 36, flexShrink: 0, pt: compact ? 0 : 0.25, display: right ? "none" : "block" }}>
        {compact ? (
          <Typography
            level="body-xs"
            className="compact-time"
            sx={{ opacity: 0, fontSize: 10, textAlign: "right", pt: 0.5 }}
          >
            {fmtClock(m.sent_at)}
          </Typography>
        ) : (
          <PersonCard userId={m.sender_id} name={name}>
            <UserAvatar id={m.sender_id} name={name} />
          </PersonCard>
        )}
      </Box>
      <Box sx={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", alignItems: right ? "flex-end" : "flex-start" }}>
        {context && (
          <Typography
            level="body-xs"
            onClick={onContextClick}
            sx={{ color: "primary.600", cursor: onContextClick ? "pointer" : "default", mb: 0.25, fontWeight: 600 }}
          >
            {context}
          </Typography>
        )}
        {!compact && (
          <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mb: 0.25 }}>
            {!right && (
              <PersonCard userId={m.sender_id} name={name}>
                <Typography level="title-sm" sx={{ fontWeight: 700 }}>
                  {name}
                </Typography>
              </PersonCard>
            )}
            <Tooltip title={new Date(m.sent_at).toLocaleString()}>
              <Typography level="body-xs" sx={{ opacity: 0.6 }}>
                {fmtClock(m.sent_at)}
              </Typography>
            </Tooltip>
            {m.pinned && <PushPinIcon sx={{ fontSize: 13, color: "warning.500" }} titleAccess="Pinned" />}
            {m.saved && <BookmarkIcon sx={{ fontSize: 13, color: "primary.500" }} titleAccess="Saved for later" />}
          </Box>
        )}

        {editing ? (
          <Box>
            <Textarea
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void saveEdit();
                } else if (e.key === "Escape") {
                  setEditing(false);
                }
              }}
              minRows={2}
              slotProps={{ textarea: { "aria-label": "Edit message" } }}
            />
            <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
              <Button size="sm" onClick={() => void saveEdit()}>
                Save
              </Button>
              <Button size="sm" variant="plain" color="neutral" onClick={() => setEditing(false)}>
                Cancel
              </Button>
              <Typography level="body-xs" sx={{ alignSelf: "center", opacity: 0.6 }}>
                Enter to save · Esc to cancel
              </Typography>
            </Box>
          </Box>
        ) : (
          <Box
            data-testid={right ? "bubble-mine" : "bubble-other"}
            onContextMenu={(e) => {
              // Long-press / right-click opens the quick-react bar on touch.
              if (window.matchMedia("(pointer: coarse)").matches) {
                e.preventDefault();
                setPickerOpen(true);
              }
            }}
            sx={{
              position: "relative",
              fontSize: "sm",
              px: 1.5,
              py: 0.75,
              maxWidth: "min(78%, 680px)",
              borderRadius: "lg",
              borderTopRightRadius: right && !compact ? 4 : undefined,
              borderTopLeftRadius: !right && !compact ? 4 : undefined,
              bgcolor: right ? "rgba(18,108,90,0.14)" : "background.level1",
              border: "1px solid",
              borderColor: right ? "rgba(18,108,90,0.28)" : "divider",
            }}
          >
            <MessageBody body={m.body} attachments={m.attachments} mentionNames={mentionNames} meName={meName} />
      {/* hover actions */}
      {!editing && (
        <Box
          className="msg-actions"
          data-testid="msg-actions"
          sx={{
            position: "absolute", bottom: compact ? "calc(100% + 2px)" : "calc(100% + 26px)", ...(right ? { right: 0 } : { left: 0 }), display: "flex", opacity: { xs: 0, md: 0 }, pointerEvents: "none", whiteSpace: "nowrap",
            bgcolor: "background.surface", border: "1px solid", borderColor: "divider", borderRadius: "md",
            boxShadow: "sm", p: 0.25, zIndex: 2,
          }}
        >
          {QUICK.slice(0, 3).map((e) => (
            <IconButton key={e} size="sm" variant="plain" onClick={() => actions.react(m, e)} aria-label={`React ${e}`}>
              {e}
            </IconButton>
          ))}
          <Tooltip title="More reactions">
            <IconButton size="sm" variant="plain" onClick={() => setPickerOpen(true)} aria-label="Add reaction">
              <AddReactionOutlinedIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
          {!inThread && actions.openThread && (
            <Tooltip title="Reply in thread">
              <IconButton size="sm" variant="plain" onClick={() => actions.openThread!(m)} aria-label="Reply in thread">
                <ModeCommentOutlinedIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title={m.saved ? "Remove from Later" : "Save for later"}>
            <IconButton size="sm" variant="plain" onClick={() => actions.save(m)} aria-label="Save for later">
              {m.saved ? <BookmarkIcon sx={{ fontSize: 18 }} /> : <BookmarkBorderIcon sx={{ fontSize: 18 }} />}
            </IconButton>
          </Tooltip>
          <Dropdown>
            <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": "More actions" } }}>
              <MoreVertIcon sx={{ fontSize: 18 }} />
            </MenuButton>
            <Menu placement="bottom-end" size="sm" sx={{ zIndex: 1500, minWidth: 200 }}>
              <MenuItem onClick={() => actions.pin(m)}>
                {m.pinned ? <PushPinIcon sx={{ fontSize: 16, mr: 1 }} /> : <PushPinOutlinedIcon sx={{ fontSize: 16, mr: 1 }} />}
                {m.pinned ? "Unpin from channel" : "Pin to channel"}
              </MenuItem>
              <MenuItem onClick={() => actions.copyLink(m)}>Copy link</MenuItem>
              <MenuItem onClick={() => actions.makeTask(m)} data-testid="chat-make-task">Make a task</MenuItem>
              {actions.makeTicket && <MenuItem onClick={() => actions.makeTicket?.(m)} data-testid="chat-make-ticket">Make a ticket</MenuItem>}
              <ListDivider />
              <Typography level="body-xs" sx={{ px: 1.5, py: 0.5, opacity: 0.6 }}>
                Remind me about this
              </Typography>
              {presets().map((p) => (
                <MenuItem key={p.label} onClick={() => actions.remind(m, p.at)}>
                  {p.label}
                </MenuItem>
              ))}
              {mine && <ListDivider />}
              {mine && (
                <MenuItem
                  onClick={() => {
                    setDraft(m.body);
                    setEditing(true);
                  }}
                >
                  Edit message
                </MenuItem>
              )}
              {mine && (
                <MenuItem color="danger" onClick={() => actions.remove(m)}>
                  Delete message
                </MenuItem>
              )}
            </Menu>
          </Dropdown>
        </Box>
      )}

            {m.edited_at && (
              <Typography level="body-xs" component="span" sx={{ opacity: 0.55, ml: 0.5 }}>
                (edited)
              </Typography>
            )}
          </Box>
        )}

        {(m.reaction_details?.length ?? 0) > 0 && (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5, justifyContent: right ? "flex-end" : "flex-start" }}>
            {m.reaction_details!.map((r) => (
              <Chip
                key={r.emoji}
                size="sm"
                variant={r.me ? "soft" : "outlined"}
                color={r.me ? "primary" : "neutral"}
                onClick={() => actions.react(m, r.emoji)}
                slotProps={{ action: { "aria-label": `${r.emoji} ${r.count}` } }}
                sx={{ cursor: "pointer" }}
              >
                {r.emoji} {r.count}
              </Chip>
            ))}
          </Box>
        )}

        {!inThread && (m.reply_count ?? 0) > 0 && actions.openThread && (
          <Box
            onClick={() => actions.openThread!(m)}
            data-testid="thread-summary"
            sx={{
              display: "inline-flex", alignItems: "center", gap: 0.75, mt: 0.5, px: 0.75, py: 0.25,
              borderRadius: "sm", cursor: "pointer", color: "primary.600",
              "&:hover": { bgcolor: "background.level2" },
            }}
          >
            <ModeCommentOutlinedIcon sx={{ fontSize: 14 }} />
            <Typography level="body-xs" sx={{ color: "inherit", fontWeight: 600 }}>
              {m.reply_count} {m.reply_count === 1 ? "reply" : "replies"}
            </Typography>
            {m.last_reply_at && (
              <Typography level="body-xs" sx={{ opacity: 0.6 }}>
                Last reply {fmtClock(m.last_reply_at)}
              </Typography>
            )}
          </Box>
        )}
      </Box>

      <Modal open={pickerOpen} onClose={() => setPickerOpen(false)}>
        <ModalDialog sx={{ p: 0, overflow: "hidden", width: "auto" }}>
          <ModalClose />
          <EmojiPicker
            data={emojiData}
            onEmojiSelect={(e: { native: string }) => {
              actions.react(m, e.native);
              setPickerOpen(false);
            }}
            theme="light"
            previewPosition="none"
          />
        </ModalDialog>
      </Modal>
    </Box>
  );
}
