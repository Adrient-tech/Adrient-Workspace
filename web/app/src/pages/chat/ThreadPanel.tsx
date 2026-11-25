import { useEffect, useMemo, useRef, useState } from "react";
import { Box, CircularProgress, Divider, IconButton, Typography } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";
import { ChatComposer } from "./ChatComposer";
import { MessageItem } from "./Message";
import { SendLaterButton } from "./Dialogs";
import { buildActions } from "./actions";
import { applyFeedToList, channelLabel } from "./lib";
import { teams } from "./teamsApi";
import type { TeamsState } from "./useTeams";
import type { TChannel, TMessage } from "./types";

interface Props {
  state: TeamsState;
  channel: TChannel;
  rootId: string;
  onClose: () => void;
  toast: (title: string, body?: string) => void;
}

export function ThreadPanel({ state, channel, rootId, onClose, toast }: Props) {
  const { me, meName, peopleMap, people } = state;
  const [parent, setParent] = useState<TMessage | null>(null);
  const [replies, setReplies] = useState<TMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const endRef = useRef<HTMLDivElement>(null);
  const mentionNames = useMemo(() => [...people.map((p) => p.name), ...people.map((p) => p.email)].filter(Boolean), [people]);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    teams
      .thread(rootId)
      .then((r) => {
        if (!alive) return;
        setParent(r.parent);
        setReplies(r.replies ?? []);
      })
      .catch(() => alive && onClose())
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [rootId, onClose]);

  useEffect(
    () =>
      state.subscribe((e) => {
        if (e.type === "message" && e.message.parent_id === rootId) {
          setReplies((cur) => (cur.some((x) => x.id === e.message.id) ? cur : [...cur, e.message]));
        } else if (e.type === "deleted" && e.id === rootId) {
          onClose();
        } else if (e.type === "edited" || e.type === "deleted" || e.type === "reaction" || e.type === "pins" || e.type === "saved") {
          setReplies((cur) => applyFeedToList(cur, e, me));
          setParent((p) => (p ? applyFeedToList([p], e, me)[0] ?? p : p));
        }
      }),
    [state, rootId, me, onClose],
  );

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [replies.length, loading]);

  const actions = useMemo(
    () =>
      buildActions({
        update: (id, patch) => {
          setReplies((cur) => cur.map((m) => (m.id === id ? { ...m, ...patch } : m)));
          setParent((p) => (p && p.id === id ? { ...p, ...patch } : p));
        },
        remove: (id) => (id === rootId ? onClose() : setReplies((cur) => cur.filter((m) => m.id !== id))),
        toast,
      }),
    [rootId, onClose, toast],
  );

  const send = async ({ body, attachmentIds }: { body: string; attachmentIds: string[] }) => {
    const m = await teams.post(channel.id, body, { parent_id: rootId, attachment_ids: attachmentIds });
    setReplies((cur) => (cur.some((x) => x.id === m.id) ? cur : [...cur, m]));
  };

  return (
    <Box
      data-testid="thread-panel"
      sx={{ width: { xs: "100%", lg: 400 }, flexShrink: 0, borderLeft: "1px solid", borderColor: "divider", display: "flex", flexDirection: "column", minHeight: 0, bgcolor: "background.surface" }}
    >
      <Box sx={{ display: "flex", alignItems: "center", px: 2, py: 1, borderBottom: "1px solid", borderColor: "divider" }}>
        <Box sx={{ flex: 1 }}>
          <Typography level="title-md">Thread</Typography>
          <Typography level="body-xs" sx={{ opacity: 0.7 }}>
            {channelLabel(channel, me, peopleMap)}
          </Typography>
        </Box>
        <IconButton size="sm" variant="plain" onClick={onClose} aria-label="Close thread">
          <CloseIcon />
        </IconButton>
      </Box>
      <Box sx={{ flex: 1, overflowY: "auto", py: 1 }}>
        {loading && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
            <CircularProgress size="sm" />
          </Box>
        )}
        {parent && (
          <>
            <MessageItem m={parent} me={me} meName={meName} people={peopleMap} mentionNames={mentionNames} actions={actions} inThread />
            <Divider sx={{ my: 1 }}>
              <Typography level="body-xs">
                {replies.length} {replies.length === 1 ? "reply" : "replies"}
              </Typography>
            </Divider>
            {replies.map((m) => (
              <MessageItem key={m.id} m={m} me={me} meName={meName} people={peopleMap} mentionNames={mentionNames} actions={actions} inThread />
            ))}
          </>
        )}
        <div ref={endRef} />
      </Box>
      {!channel.archived && channel.is_member && (
        <Box sx={{ px: 1.5, pb: 1.5, pt: 0.5 }}>
          <ChatComposer
            key={rootId}
            placeholder="Reply…"
            onSend={send}
            extraActions={({ value, clear }) => (
              <SendLaterButton
                value={value}
                onSchedule={async (at) => {
                  try {
                    await teams.schedule(channel.id, value.trim(), at.toISOString(), rootId);
                    clear();
                    void state.refreshCounts();
                    toast("Reply scheduled", at.toLocaleString());
                  } catch (e) {
                    toast("Could not schedule", (e as Error).message);
                  }
                }}
              />
            )}
          />
        </Box>
      )}
    </Box>
  );
}
