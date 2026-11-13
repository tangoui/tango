import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    // "@/components/ui/button" → src/components/ui/button (also in tsconfig.json
    // and tsconfig.app.json, and the aliases in components.json).
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  // A fixed port: if it is taken, Vite stops with a clear error instead of
  // moving to another port that the HMR client would not follow.
  server: { port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
})
