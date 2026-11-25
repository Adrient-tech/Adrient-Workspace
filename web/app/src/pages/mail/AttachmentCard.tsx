import { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Button,
  Dropdown,
  IconButton,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Sheet,
  Typography,
  ListItemDecorator,
} from "@mui/joy";
import * as Icons from "@mui/icons-material";
import { AppIcon } from "../../components/AppIcon";
import { apps } from "../../catalog/apps";
import { appForFile, appLabel, openDriveFile, withReturn } from "../../lib/openWith";
import { DrivePicker } from "../drive/DrivePicker";
import {
  attachmentURL,
  saveAttachmentToDrive,
  type SavedDriveFile,
} from "./api";
import type { MailAttachment } from "./types";

/** Preview kind for attachments the browser can show inline. */
export function previewKind(a: Pick<MailAttachment, "filename" | "content_type">): "image" | "pdf" | null {
  const ct = (a.content_type ?? "").toLowerCase();
  if (/^image\/(png|jpe?g|gif|webp)$/.test(ct) || /\.(png|jpe?g|gif|webp)$/i.test(a.filename)) return "image";
  if (ct === "application/pdf" || /\.pdf$/i.test(a.filename)) return "pdf";
  return null;
}

/**
 * One mail attachment: the app icon for its type plus Open (in the right
 * Adrient editor via a Drive copy), Save to Drive, Preview and Download.
 */
export function AttachmentCard({
  messageId,
  att,
  onNotice,
}: {
  messageId: string;
  att: MailAttachment;
  onNotice?: (msg: string) => void;
}) {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [picker, setPicker] = useState(false);
  const [preview, setPreview] = useState(false);
  const appId = appForFile({ name: att.filename, mime_type: att.content_type });
  const tile = apps.find((x) => x.id === appId) ?? apps.find((x) => x.id === "drive");
  const kind = previewKind(att);
  const sizeLabel = att.size >= 1048576 ? `${(att.size / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(att.size / 1024))} KB`;

  async function open() {
    setBusy(true);
    try {
      const f = await saveAttachmentToDrive(messageId, att.id);
      const t = openDriveFile({ name: f.name, mime_type: f.mime_type, drive_id: f.id });
      navigate(withReturn(t.href, "/mail"));
    } catch (e) {
      onNotice?.(`Could not open: ${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  async function saveTo(folderId: string) {
    setPicker(false);
    setBusy(true);
    try {
      const f: SavedDriveFile = await saveAttachmentToDrive(messageId, att.id, folderId || "root");
      onNotice?.(`Saved “${f.name}” to Drive`);
    } catch (e) {
      onNotice?.(`Save failed: ${(e as Error).message}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Sheet
      variant="outlined"
      data-testid={`mail-att-${att.filename}`}
      sx={{ display: "flex", alignItems: "center", gap: 1, p: 0.75, pr: 0.5, borderRadius: "md", maxWidth: 360 }}
    >
      {tile && <AppIcon iconName={tile.iconName} color={tile.accentColor} size={32} label={appLabel(appId)} />}
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography level="body-sm" noWrap title={att.filename}>
          {att.filename}
        </Typography>
        <Typography level="body-xs" sx={{ opacity: 0.6 }}>
          {sizeLabel}
        </Typography>
      </Box>
      <Button size="sm" variant="soft" loading={busy} onClick={open} data-testid="att-open">
        Open
      </Button>
      <Dropdown>
        <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": "Attachment actions" } }}>
          <Icons.MoreVert fontSize="small" />
        </MenuButton>
        <Menu placement="bottom-end" size="sm">
          <MenuItem onClick={() => setPicker(true)} data-testid="att-save-drive">
            <ListItemDecorator><Icons.AddToDrive fontSize="small" /></ListItemDecorator>
            Save to Drive…
          </MenuItem>
          {kind && (
            <MenuItem onClick={() => setPreview(true)} data-testid="att-preview">
              <ListItemDecorator><Icons.Visibility fontSize="small" /></ListItemDecorator>
              Preview
            </MenuItem>
          )}
          <MenuItem component="a" href={attachmentURL(att.id)} download={att.filename}>
            <ListItemDecorator><Icons.Download fontSize="small" /></ListItemDecorator>
            Download
          </MenuItem>
        </Menu>
      </Dropdown>
      {picker && (
        <DrivePicker
          mode="folder"
          title="Save to Drive"
          confirmLabel="Save here"
          onClose={() => setPicker(false)}
          onPick={(p) => void saveTo(p.id)}
        />
      )}
      {preview && kind && (
        <Modal open onClose={() => setPreview(false)}>
          <ModalDialog sx={{ width: "min(900px, 92vw)", height: "85vh", p: 1 }} data-testid="att-preview-dialog">
            <ModalClose />
            <Typography level="title-sm" sx={{ px: 1 }}>{att.filename}</Typography>
            {kind === "image" ? (
              <Box component="img" src={`${attachmentURL(att.id)}?inline=1`} alt={att.filename} sx={{ maxWidth: "100%", maxHeight: "100%", objectFit: "contain", m: "auto" }} />
            ) : (
              <Box component="iframe" title={att.filename} src={`${attachmentURL(att.id)}?inline=1`} sx={{ flex: 1, border: 0, width: "100%" }} />
            )}
          </ModalDialog>
        </Modal>
      )}
    </Sheet>
  );
}
