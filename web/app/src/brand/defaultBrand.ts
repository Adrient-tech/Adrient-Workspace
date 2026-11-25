/** Brand is the per-deploy theming surface. Each field is a CSS-ready value. */
export interface Brand {
  productName: string;
  tagline: string;
  primaryColor: string;
  surfaceColor: string;
  onSurfaceColor: string;
  // Optional logo SVG markup. If absent, the productName initial is shown.
  logoSVG?: string;
  supportURL: string;
}

// The default workspace mark: the owner's Adrient mark (public/adrient-mark.png)
// on a white tile so it reads on both light and dark surfaces. The artwork is
// never redrawn or recoloured.
export const workspaceLogoSVG = `<div role="img" aria-label="Adrient" style="width:100%;height:100%;box-sizing:border-box;padding:12%;background:#fff;border-radius:inherit;display:flex;align-items:center;justify-content:center;box-shadow:inset 0 0 0 1px rgba(18,108,90,.14)"><img src="/adrient-mark.png" alt="" style="width:100%;height:100%;object-fit:contain" /></div>`;

export const defaultBrand: Brand = {
  productName: "Adrient Workspace",
  tagline: "Your own workspace. Your data. Your infrastructure.",
  primaryColor: "#126C5A", // Adrient brand green
  surfaceColor: "#F7FAF9", // soft green-tinted white
  onSurfaceColor: "#00261F", // Adrient ink (dark green)
  logoSVG: workspaceLogoSVG,
  supportURL: "https://adrient.com/support",
};
