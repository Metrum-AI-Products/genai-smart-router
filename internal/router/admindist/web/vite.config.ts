// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import path from "node:path";

export default defineConfig({
  base: "./",
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "..",
    emptyOutDir: false,
    assetsDir: "assets",
    rollupOptions: {
      output: {
        entryFileNames: "static/assets/admin-[hash].js",
        chunkFileNames: "static/assets/admin-[hash].js",
        assetFileNames: "static/assets/admin-[hash][extname]",
      },
    },
  },
});
