import { useState } from "react";
import { Box, Dropdown, IconButton, Input, Menu, MenuButton, MenuItem, Typography } from "@mui/joy";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import type { DeckSection } from "./sections";

export interface SectionCommands {
  rename: (id: string, name: string) => void;
  remove: (id: string) => void;
  removeWithSlides: (id: string) => void;
  move: (id: string, dir: -1 | 1) => void;
}

/** Heading above the first slide of a section in the slide rail. */
export function SectionBar({ section, count, cmd }: { section: DeckSection; count: number; cmd: SectionCommands }) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(section.name);
  const commit = () => {
    setEditing(false);
    if (name.trim() && name !== section.name) cmd.rename(section.id, name);
    else setName(section.name);
  };
  return (
    <Box
      data-testid="section-bar"
      onClick={(e) => e.stopPropagation()}
      sx={{ display: "flex", alignItems: "center", gap: 0.5, mb: 0.5, mt: 0.5, px: 0.5, borderBottom: "2px solid", borderColor: "primary.500" }}
    >
      {editing ? (
        <Input
          autoFocus
          size="sm"
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") commit();
            if (e.key === "Escape") {
              setName(section.name);
              setEditing(false);
            }
          }}
          slotProps={{ input: { "aria-label": "Section name" } }}
          sx={{ flex: 1, minWidth: 0 }}
        />
      ) : (
        <Typography level="title-sm" noWrap sx={{ flex: 1, cursor: "text" }} onDoubleClick={() => setEditing(true)} title="Double-click to rename">
          {section.name} <Typography level="body-xs">({count})</Typography>
        </Typography>
      )}
      <Dropdown>
        <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": `Section ${section.name} options` } }}>
          <MoreVertIcon fontSize="small" />
        </MenuButton>
        <Menu size="sm" sx={{ zIndex: 1300 }}>
          <MenuItem onClick={() => setEditing(true)}>Rename section</MenuItem>
          <MenuItem onClick={() => cmd.move(section.id, -1)}>Move section up</MenuItem>
          <MenuItem onClick={() => cmd.move(section.id, 1)}>Move section down</MenuItem>
          <MenuItem onClick={() => cmd.remove(section.id)}>Remove section</MenuItem>
          <MenuItem color="danger" onClick={() => cmd.removeWithSlides(section.id)}>
            Remove section and its slides
          </MenuItem>
        </Menu>
      </Dropdown>
    </Box>
  );
}
