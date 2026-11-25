import { Box, Link } from "@mui/joy";
import { APP_VERSION, PRODUCT_NAME, PRODUCT_URL } from "./version";

/** ProductBadge is the small "Adrient Workspace v1.0.0" tag that links to
 *  adrient.com in a new tab. `corner` pins it to the bottom right of the
 *  viewport (launcher screens); `line` is a quiet centred line for sign-in and
 *  public pages. */
export function ProductBadge({ variant = "corner" }: { variant?: "corner" | "line" }) {
  const link = (
    <Link
      href={PRODUCT_URL}
      target="_blank"
      rel="noopener noreferrer"
      underline="none"
      data-testid="product-badge-link"
      aria-label={`${PRODUCT_NAME} version ${APP_VERSION}, opens adrient.com in a new tab`}
      sx={{
        display: "inline-flex",
        alignItems: "center",
        gap: 0.75,
        color: "text.tertiary",
        fontSize: "0.75rem",
        lineHeight: 1.4,
        whiteSpace: "nowrap",
        "&:hover": { color: "primary.plainColor" },
      }}
    >
      <Box component="img" src="/adrient-mark.png" alt="" sx={{ width: 14, height: 14, display: "block" }} />
      <span>{PRODUCT_NAME}</span>
      <Box component="span" sx={{ opacity: 0.8 }}>v{APP_VERSION}</Box>
    </Link>
  );
  if (variant === "line") {
    return (
      <Box data-testid="product-badge" sx={{ display: "flex", justifyContent: "center", py: 1.5 }}>
        {link}
      </Box>
    );
  }
  return (
    <Box
      data-testid="product-badge"
      sx={{
        position: "fixed",
        right: 12,
        bottom: "calc(8px + env(safe-area-inset-bottom, 0px))",
        zIndex: 5,
        px: 1.25,
        py: 0.5,
        borderRadius: "999px",
        bgcolor: "background.surface",
        border: "1px solid",
        borderColor: "divider",
        boxShadow: "var(--joy-shadow-sm)",
        opacity: 0.95,
      }}
    >
      {link}
    </Box>
  );
}

/** PublicBrandLine sits at the bottom centre of a public page (careers,
 *  booking, share pages) without touching the page's own layout. */
export function PublicBrandLine() {
  return (
    <Box
      sx={{
        position: "fixed",
        left: 0,
        right: 0,
        bottom: "calc(4px + env(safe-area-inset-bottom, 0px))",
        zIndex: 5,
        display: "flex",
        justifyContent: "center",
        pointerEvents: "none",
        "& a": { pointerEvents: "auto", bgcolor: "rgba(255,255,255,.8)", px: 1, borderRadius: "999px" },
      }}
    >
      <ProductBadge variant="line" />
    </Box>
  );
}
