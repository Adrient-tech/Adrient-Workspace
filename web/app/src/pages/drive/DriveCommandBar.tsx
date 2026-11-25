import { Box, Button, Divider, Dropdown, Menu, MenuButton, MenuItem, Tooltip } from "@mui/joy";
import * as Icons from "@mui/icons-material";
import type { DriveFile } from "./types";
import { isFolder } from "./types";
import type { RowMenuHandlers } from "./RowMenuItems";
import type { DriveGroup, DriveSort } from "./driveSort";

function Cmd({ icon, label, onClick, disabled, danger }: { icon: React.ReactNode; label: string; onClick?: () => void; disabled?: boolean; danger?: boolean }) {
  return (
    <Tooltip title={label} enterDelay={600}>
      <span>
        <Button size="sm" variant="plain" color={danger ? "danger" : "neutral"} disabled={disabled} onClick={onClick} startDecorator={icon} aria-label={label} sx={{ flexShrink: 0, whiteSpace: "nowrap", px: 1 }}>
          <Box component="span" sx={{ display: { xs: "none", xl: "inline" } }}>{label}</Box>
        </Button>
      </span>
    </Tooltip>
  );
}
const Sep = () => <Divider orientation="vertical" sx={{ mx: 0.5, my: 0.5, flexShrink: 0 }} />;

export type DriveLayout = "list" | "grid" | "gallery";

export interface DriveCommandBarProps {
  file: DriveFile | null;
  handlers: RowMenuHandlers;
  canCreate: boolean;
  trashView: boolean;
  onNewFolder: () => void;
  onUpload: () => void;
  onRestore: (id: string) => void;
  onDeleteForever: (id: string) => void;
  sort: DriveSort;
  desc: boolean;
  group: DriveGroup;
  setSort: (s: DriveSort, desc: boolean) => void;
  setGroup: (g: DriveGroup) => void;
  view: DriveLayout;
  setView: (v: DriveLayout) => void;
}

const SORT_LABEL: Record<DriveSort, string> = { name: "Name", modified: "Modified", size: "File size", type: "File type" };
const GROUP_LABEL: Record<DriveGroup, string> = { none: "None", type: "File type", modified: "Date modified" };

/** OneDrive/SharePoint-style command bar. Acts on the selected item; every command maps to an existing handler. */
export function DriveCommandBar(p: DriveCommandBarProps) {
  const f = p.file;
  const file = f && !isFolder(f) ? f : null;
  return (
    <Box data-testid="drive-command-bar" sx={{ display: "flex", alignItems: "center", gap: 0.25, overflowX: "auto", py: 0.5, mb: 1, borderBottom: "1px solid", borderColor: "divider" }}>
      {p.canCreate && (
        <>
          <Dropdown>
            <MenuButton size="sm" variant="solid" color="primary" startDecorator={<Icons.Add />} sx={{ flexShrink: 0 }}>New</MenuButton>
            <Menu size="sm" placement="bottom-start" sx={{ zIndex: 1500 }}>
              <MenuItem onClick={p.onNewFolder}><Icons.CreateNewFolder sx={{ mr: 1 }} />Folder</MenuItem>
              <MenuItem onClick={p.onUpload}><Icons.CloudUpload sx={{ mr: 1 }} />File upload</MenuItem>
            </Menu>
          </Dropdown>
          <Cmd icon={<Icons.Upload />} label="Upload" onClick={p.onUpload} />
          <Sep />
        </>
      )}
      {p.trashView ? (
        <>
          <Cmd icon={<Icons.RestoreFromTrash />} label="Restore" disabled={!f} onClick={() => f && p.onRestore(f.id)} />
          <Cmd icon={<Icons.DeleteForever />} label="Delete forever" danger disabled={!f} onClick={() => f && p.onDeleteForever(f.id)} />
        </>
      ) : (
        <>
          <Cmd icon={<Icons.Share />} label="Share" disabled={!f} onClick={() => f && p.handlers.onShareOpenPanel(f)} />
          <Cmd icon={<Icons.Link />} label="Copy link" disabled={!f} onClick={() => f && p.handlers.onCopyLink(f)} />
          <Cmd icon={<Icons.Download />} label="Download" disabled={!file} onClick={() => file && p.handlers.onDownload(file)} />
          <Cmd icon={<Icons.DeleteOutline />} label="Delete" danger disabled={!f} onClick={() => f && p.handlers.onTrash(f.id)} />
          <Sep />
          <Cmd icon={<Icons.DriveFileMove />} label="Move to" disabled={!f} onClick={() => f && p.handlers.onMove(f)} />
          <Cmd icon={<Icons.ContentCopy />} label="Make a copy" disabled={!file} onClick={() => file && p.handlers.onMakeCopy(file)} />
          <Cmd icon={<Icons.DriveFileRenameOutline />} label="Rename" disabled={!f} onClick={() => f && p.handlers.onRename(f)} />
          <Cmd icon={<Icons.StarBorder />} label="Favourite" disabled={!f} onClick={() => f && p.handlers.onToggleStar(f)} />
          <Sep />
          <Cmd icon={<Icons.InfoOutlined />} label="Details" disabled={!f} onClick={() => f && p.handlers.onShowDetails(f)} />
          <Cmd icon={<Icons.History />} label="Version history" disabled={!file} onClick={() => file && p.handlers.onManageVersions(file)} />
        </>
      )}
      <Box sx={{ flex: 1 }} />
      <Dropdown>
        <MenuButton size="sm" variant="plain" color="neutral" startDecorator={<Icons.Sort />} sx={{ flexShrink: 0 }} aria-label="Sort">
          <Box component="span" sx={{ display: { xs: "none", xl: "inline" } }}>Sort: {SORT_LABEL[p.sort]} {p.desc ? "↓" : "↑"}</Box>
        </MenuButton>
        <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1500 }}>
          {(Object.keys(SORT_LABEL) as DriveSort[]).map((k) => (
            <MenuItem key={k} selected={p.sort === k} onClick={() => p.setSort(k, p.sort === k ? !p.desc : k === "modified")}>
              {SORT_LABEL[k]}
            </MenuItem>
          ))}
        </Menu>
      </Dropdown>
      <Dropdown>
        <MenuButton size="sm" variant="plain" color="neutral" startDecorator={<Icons.ViewAgenda />} sx={{ flexShrink: 0 }} aria-label="Group by">
          <Box component="span" sx={{ display: { xs: "none", xl: "inline" } }}>Group: {GROUP_LABEL[p.group]}</Box>
        </MenuButton>
        <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1500 }}>
          {(Object.keys(GROUP_LABEL) as DriveGroup[]).map((k) => (
            <MenuItem key={k} selected={p.group === k} onClick={() => p.setGroup(k)}>{GROUP_LABEL[k]}</MenuItem>
          ))}
        </Menu>
      </Dropdown>
      <Dropdown>
        <MenuButton size="sm" variant="plain" color="neutral" startDecorator={p.view === "list" ? <Icons.ViewList /> : p.view === "grid" ? <Icons.GridView /> : <Icons.Collections />} sx={{ flexShrink: 0 }} aria-label="Layout">
          <Box component="span" sx={{ display: { xs: "none", xl: "inline" } }}>{p.view[0].toUpperCase() + p.view.slice(1)}</Box>
        </MenuButton>
        <Menu size="sm" placement="bottom-end" sx={{ zIndex: 1500 }}>
          <MenuItem selected={p.view === "list"} onClick={() => p.setView("list")}>List</MenuItem>
          <MenuItem selected={p.view === "grid"} onClick={() => p.setView("grid")}>Tiles</MenuItem>
          <MenuItem selected={p.view === "gallery"} onClick={() => p.setView("gallery")}>Gallery</MenuItem>
        </Menu>
      </Dropdown>
    </Box>
  );
}
