import type { ReactNode } from "react";
import { Box, Typography } from "@mui/joy";

export function SettingsSection({ title, hint, action, children }: { title: string; hint: string; action?: ReactNode; children: ReactNode }) {
  return (
    <Box sx={{ maxWidth: 940 }}>
      <Box sx={{ display: "flex", alignItems: "flex-start", gap: 2, mb: 2 }}>
        <Box sx={{ flex: 1 }}>
          <Typography level="h4">{title}</Typography>
          <Typography level="body-sm" sx={{ color: "text.tertiary", maxWidth: 640 }}>{hint}</Typography>
        </Box>
        {action}
      </Box>
      {children}
    </Box>
  );
}

