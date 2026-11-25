import type { ReactNode } from "react";
import { Box, DialogContent, DialogTitle, Link, Modal, ModalClose, ModalDialog, Typography } from "@mui/joy";
import { APP_BUILD, APP_VERSION, LICENCE, PRODUCT_NAME, PRODUCT_URL, SUPPORT_EMAIL } from "./version";

/** AboutDialog is opened from the account menu: product name, version, build,
 *  website, support address and licence. */
export function AboutDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const rows: [string, ReactNode][] = [
    ["Version", <span data-testid="about-version">{APP_VERSION}</span>],
    ["Build", <span data-testid="about-build">{APP_BUILD}</span>],
    ["Website", <Link href={PRODUCT_URL} target="_blank" rel="noopener noreferrer">adrient.com</Link>],
    ["Support", <Link href={`mailto:${SUPPORT_EMAIL}`}>{SUPPORT_EMAIL}</Link>],
    ["Licence", `${LICENCE} (open source)`],
  ];
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog data-testid="about-dialog" aria-labelledby="about-title" sx={{ width: 380, maxWidth: "calc(100vw - 32px)" }}>
        <ModalClose />
        <Box sx={{ display: "flex", justifyContent: "center", mb: 1 }}>
          <Box
            component="img"
            src="/adrient-logo.png"
            alt="Adrient"
            sx={{ height: 40, maxWidth: "80%", objectFit: "contain", display: "block", "[data-joy-color-scheme=dark] &": { display: "none" } }}
          />
          <Box
            component="img"
            src="/adrient-logo-white.png"
            alt="Adrient"
            sx={{ height: 40, maxWidth: "80%", objectFit: "contain", display: "none", "[data-joy-color-scheme=dark] &": { display: "block" } }}
          />
        </Box>
        <DialogTitle id="about-title" sx={{ justifyContent: "center" }}>
          {PRODUCT_NAME}
        </DialogTitle>
        <DialogContent>
          <Box component="dl" sx={{ display: "grid", gridTemplateColumns: "auto 1fr", columnGap: 2, rowGap: 0.75, m: 0 }}>
            {rows.map(([k, v]) => (
              <Box key={k} sx={{ display: "contents" }}>
                <Typography component="dt" level="body-sm" sx={{ color: "text.tertiary" }}>{k}</Typography>
                <Typography component="dd" level="body-sm" sx={{ m: 0, textAlign: "right" }}>{v}</Typography>
              </Box>
            ))}
          </Box>
          <Typography level="body-xs" sx={{ mt: 2, color: "text.tertiary", textAlign: "center" }}>
            Your own workspace. Your data. Your infrastructure.
          </Typography>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}
