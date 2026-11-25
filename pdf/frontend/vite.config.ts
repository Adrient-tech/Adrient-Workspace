import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

// When integrated under adrient-workspace the app is served at /pdf/; standalone
// dev keeps "/". Set ADRIENT_PDF_BASE=/pdf/ for the integrated build/dev.
const base = process.env.ADRIENT_PDF_BASE || "/";

export default defineConfig({
  base,
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    // Bind all interfaces (IPv4 + IPv6) so adrient's reverse proxy can dial
    // 127.0.0.1; default "localhost" binds IPv6-only and the proxy 502s.
    host: true,
    // Allow being reached through adrient's reverse proxy (Vite 6 host check).
    allowedHosts: true,
    proxy: {
      "/api": {
        target: "http://localhost:8085",
        changeOrigin: false, // Keep original host to preserve cookies
      },
      "/auth": {
        target: "http://localhost:8085",
        changeOrigin: false,
      },
    },
  },
});
