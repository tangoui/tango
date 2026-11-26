import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  // Use one loopback address so strictPort checks the endpoint tan dev uses.
  server: { host: "127.0.0.1", port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
});
