import { Avatar, Box, Chip, Divider, Drawer, ModalClose, Typography } from "@mui/joy";
import type { Group, GroupMember } from "../groups/types";

/** Side panel with a group's details and its members. */
export function GroupPanel({ group, members, onClose, actions }: { group: Group | null; members: GroupMember[]; onClose: () => void; actions?: React.ReactNode }) {
  const byId = new Map(members.map((m) => [m.id, m]));
  const people = (group?.member_ids ?? []).map((id) => byId.get(id)).filter((m): m is GroupMember => !!m);
  const unknown = (group?.member_ids?.length ?? 0) - people.length;
  return (
    <Drawer open={!!group} onClose={onClose} anchor="right" size="md" slotProps={{ content: { sx: { width: { xs: "100%", sm: 420 }, p: 0 } } }}>
      {group && (
        <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }} data-testid="group-panel">
          <Box sx={{ p: 2.5, pb: 1.5, display: "flex", gap: 1.5, alignItems: "center" }}>
            <Avatar size="lg">{(group.name || group.email || "?").charAt(0).toUpperCase()}</Avatar>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography level="title-lg" noWrap>{group.name || "(unnamed group)"}</Typography>
              <Typography level="body-sm" sx={{ color: "text.tertiary" }} noWrap>{group.email || "No group address"}</Typography>
            </Box>
            <ModalClose sx={{ position: "static" }} />
          </Box>
          {actions && <Box sx={{ px: 2.5, pb: 1.5, display: "flex", gap: 1 }}>{actions}</Box>}
          <Divider />
          <Box sx={{ flex: 1, overflow: "auto", p: 2.5, display: "flex", flexDirection: "column", gap: 2 }}>
            {group.description && <Typography level="body-sm">{group.description}</Typography>}
            <Box sx={{ display: "flex", gap: 1 }}>
              <Chip size="sm" variant="soft">{group.member_count} members</Chip>
              <Chip size="sm" variant="soft">{group.topic_count} topics</Chip>
              <Chip size="sm" variant="soft">{group.post_count} posts</Chip>
            </Box>
            <Box>
              <Typography level="title-sm" sx={{ mb: 0.75 }}>Members</Typography>
              {people.length === 0 && <Typography level="body-sm" sx={{ color: "text.tertiary" }}>No members yet.</Typography>}
              {people.map((m) => (
                <Box key={m.id} sx={{ display: "flex", gap: 1.25, alignItems: "center", py: 0.6 }}>
                  <Avatar size="sm">{(m.name || m.email).charAt(0).toUpperCase()}</Avatar>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography level="body-sm" noWrap>{m.name || m.email}</Typography>
                    <Typography level="body-xs" sx={{ color: "text.tertiary" }} noWrap>{m.email}</Typography>
                  </Box>
                </Box>
              ))}
              {unknown > 0 && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{unknown} more not shown (people outside the directory).</Typography>}
            </Box>
            <Typography level="body-xs" sx={{ color: "text.tertiary" }}>Created {new Date(group.created_at).toLocaleDateString()}</Typography>
          </Box>
        </Box>
      )}
    </Drawer>
  );
}
