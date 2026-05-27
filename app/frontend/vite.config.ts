import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    host: "0.0.0.0",
    strictPort: true,
    proxy: {
      "/api/ws": {
        target: process.env.VITE_PROXY_TARGET || "http://127.0.0.1:3010",
        ws: true,
      },
      "/api": {
        target: process.env.VITE_PROXY_TARGET || "http://127.0.0.1:3010",
      },
      "/health": {
        target: process.env.VITE_PROXY_TARGET || "http://127.0.0.1:3010",
      },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
});
