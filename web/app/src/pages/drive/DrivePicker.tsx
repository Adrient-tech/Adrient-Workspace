import { useEffect, useState } from "react";
import {
  Modal,
  ModalDialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  List,
  ListItem,
  ListItemButton,
  ListItemDecorator,
  CircularProgress,
} from "@mui/joy";
import * as Icons from "@mui/icons-material";
import { listFiles } from "./api";
import type { DriveFile } from "./types";
import { isFolder } from "./types";

interface Crumb {
  id: string; // "" = root
  name: string;
}

export interface DrivePickerProps {
  /** "file" picks a file (folders are navigated); "folder" picks the open folder. */
  mode: "file" | "folder";
  title?: string;
  confirmLabel?: string;
  onPick: (picked: DriveFile | { id: string; name: string }) => void;
  onClose: () => void;
}

/** Shared Drive file/folder picker used by Mail, Calendar and Chat hand-offs. */
export function DrivePicker({
  mode,
  title,
  confirmLabel,
  onPick,
  onClose,
}: DrivePickerProps) {
  const [crumbs, setCrumbs] = useState<Crumb[]>([{ id: "", name: "My Drive" }]);
  const [items, setItems] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const current = crumbs[crumbs.length - 1];

  useEffect(() => {
    let live = true;
    setLoading(true);
    setError(null);
    listFiles(current.id)
      .then((all) => {
        if (live) setItems(mode === "folder" ? all.filter(isFolder) : all);
      })
      .catch((e) => live && setError((e as Error).message))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [current.id, mode]);

  return (
    <Modal open onClose={onClose}>
      <ModalDialog
        sx={{ width: 460, maxWidth: "90vw" }}
        data-testid="drive-picker"
      >
        <DialogTitle>
          {mode === "folder" ? <Icons.Folder /> : <Icons.AddToDrive />}
          {title ?? (mode === "folder" ? "Choose a folder" : "Choose a file from Drive")}
        </DialogTitle>
        <DialogContent sx={{ p: 0 }}>
          <Box sx={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 0.5, mb: 1 }}>
            {crumbs.map((c, i) => (
              <Box key={c.id || "root"} sx={{ display: "flex", alignItems: "center" }}>
                {i > 0 && <Icons.ChevronRight fontSize="small" sx={{ opacity: 0.5 }} />}
                <Button
                  variant="plain"
                  size="sm"
                  color="neutral"
                  disabled={i === crumbs.length - 1}
                  onClick={() => setCrumbs((cs) => cs.slice(0, i + 1))}
                  sx={{ minHeight: 0, py: 0.25, px: 0.75 }}
                >
                  {c.name}
                </Button>
              </Box>
            ))}
          </Box>
          <Box sx={{ maxHeight: 320, overflow: "auto" }}>
            {loading ? (
              <Box sx={{ display: "flex", justifyContent: "center", p: 3 }}>
                <CircularProgress size="sm" />
              </Box>
            ) : error ? (
              <Typography color="danger" level="body-sm" sx={{ p: 1 }}>
                {error}
              </Typography>
            ) : items.length === 0 ? (
              <Typography level="body-sm" sx={{ p: 2, opacity: 0.6 }}>
                {mode === "folder" ? "No subfolders" : "Empty folder"}
              </Typography>
            ) : (
              <List size="sm">
                {items.map((f) => (
                  <ListItem key={f.id}>
                    <ListItemButton
                      data-testid={`picker-item-${f.name}`}
                      onClick={() =>
                        isFolder(f)
                          ? setCrumbs((cs) => [...cs, { id: f.id, name: f.name }])
                          : onPick(f)
                      }
                    >
                      <ListItemDecorator>
                        {isFolder(f) ? <Icons.Folder /> : <Icons.InsertDriveFileOutlined />}
                      </ListItemDecorator>
                      {f.name}
                    </ListItemButton>
                  </ListItem>
                ))}
              </List>
            )}
          </Box>
        </DialogContent>
        <DialogActions>
          {mode === "folder" && (
            <Button
              data-testid="picker-confirm"
              onClick={() => onPick({ id: current.id, name: current.name })}
            >
              {confirmLabel ?? "Save here"}
            </Button>
          )}
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}
