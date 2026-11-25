/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_GIPHY_KEY?: string;
}

// eslint-disable-next-line @typescript-eslint/no-empty-object-type
interface ImportMeta {
  readonly env: ImportMetaEnv;
}

/** Injected by vite.config.ts from the repo-root VERSION file and git. */
declare const __ADRIENT_VERSION__: string;
declare const __ADRIENT_BUILD__: string;
