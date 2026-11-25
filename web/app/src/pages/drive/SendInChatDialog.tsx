import { useEffect, useState } from "react";
import {
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItem,
  ListItemButton,
  ListItemDecorator,
  Modal,
  ModalDialog,
  Typography,
} from "@mui/joy";
import * as Icons from "@mui/icons-material";
import { teams } from "../chat/teamsApi";
import { channelLabel, type PeopleMap } from "../chat/lib";
import type { TChannel } from "../chat/types";
import type { DriveFile } from "./types";
import { openFileRoute } from "./editorRoutes";

/** Picks a channel or DM and posts a file card (name + in-app link) to it. */
export function SendInChatDialog({
  file,
  onSent,
  onClose,
}: {
  file: DriveFile;
  onSent: (channel: TChannel) => void;
  onClose: () => void;
}) {
  const [channels, setChannels] = useState<TChannel[] | null>(null);
  const [people, setPeople] = useState<PeopleMap>({});
  const [me, setMe] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let live = true;
    Promise.all([teams.bootstrap(), teams.people()])
      .then(([b, ps]) => {
        if (!live) return;
        setChannels(b.channels.filter((c) => !c.archived));
        setMe(b.me);
        setPeople(Object.fromEntries(ps.map((p) => [p.id, p])));
      })
      .catch((e) => live && setError((e as Error).message));
    return () => {
      live = false;
    };
  }, []);

  async function send(c: TChannel) {
    setBusy(true);
    try {
      const url = `${window.location.origin}${openFileRoute(file)}`;
      await teams.post(c.id, `📎 ${file.name}\n${url}`);
      onSent(c);
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420, maxWidth: "90vw" }} data-testid="send-chat-dialog">
        <DialogTitle>
          <Icons.ChatBubbleOutline />
          Send “{file.name}” in Chat
        </DialogTitle>
        <DialogContent sx={{ maxHeight: 340 }}>
          {error ? (
            <Typography color="danger" level="body-sm">{error}</Typography>
          ) : channels === null ? (
            <CircularProgress size="sm" />
          ) : channels.length === 0 ? (
            <Typography level="body-sm">No channels or conversations yet.</Typography>
          ) : (
            <List size="sm">
              {channels.map((c) => (
                <ListItem key={c.id}>
                  <ListItemButton disabled={busy} onClick={() => void send(c)} data-testid={`chat-target-${c.id}`}>
                    <ListItemDecorator>
                      {c.kind === "public" || c.kind === "private" ? <Icons.Tag fontSize="small" /> : <Icons.Person fontSize="small" />}
                    </ListItemDecorator>
                    {channelLabel(c, me, people)}
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}
