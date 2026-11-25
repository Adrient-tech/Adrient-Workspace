import type { ReactNode } from "react";
import { Box, Typography } from "@mui/joy";

interface PageHeaderProps {
  title: string;
  subtitle?: ReactNode;
  /** Leading element, typically an <AppIcon/>. */
  icon?: ReactNode;
  /** Right-aligned actions. */
  actions?: ReactNode;
}

/** PageHeader is the standard title row for a page or a list view: icon,
 *  display-face title, one muted line, actions on the right (wraps on phones). */
export function PageHeader({ title, subtitle, icon, actions }: PageHeaderProps) {
  return (
    <Box
      component="header"
      data-testid="page-header"
      sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", mb: 2.5 }}
    >
      {icon}
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography level="h3" component="h1" noWrap>
          {title}
        </Typography>
        {subtitle && (
          <Typography level="body-sm" sx={{ color: "text.tertiary" }}>
            {subtitle}
          </Typography>
        )}
      </Box>
      {actions && <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>{actions}</Box>}
    </Box>
  );
}
