import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // A fixed port: if it is taken, Vite stops with a clear error instead of
  // moving to another port that the HMR client would not follow.
  server: { port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
});
