import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { installFonts } from "./lib/fonts";
import "./styles/adrient.css";

// The shared open-font bundle and Calibri/Arial/… fallbacks (CC7).
installFonts();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);

// Fade out the pre-JS splash (index.html) once React has painted; the in-app
// AppSplash has the same look, so the hand-off is seamless.
requestAnimationFrame(() =>
  requestAnimationFrame(() => {
    const el = document.getElementById("pre-splash");
    if (!el) return;
    el.classList.add("out");
    setTimeout(() => el.remove(), 300);
  }),
);

// Register the service worker so the app is installable and works offline on
// phones (cached shell + hashed assets). Navigation stays network-first while
// online, so updates always flow; the cache only serves when offline.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {
      /* PWA is progressive enhancement — ignore registration failures */
    });
  });
}
