import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  // A fixed port: if it is taken, Vite stops with a clear error instead of
  // moving to another port that the HMR client would not follow.
  server: { port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
});
