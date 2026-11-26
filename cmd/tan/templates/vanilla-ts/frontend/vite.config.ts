import { defineConfig } from "vite";

export default defineConfig({
  // Use one loopback address so strictPort checks the endpoint tan dev uses.
  server: { host: "127.0.0.1", port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
});
