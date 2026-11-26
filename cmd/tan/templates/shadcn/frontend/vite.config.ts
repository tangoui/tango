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
  // Use one loopback address so strictPort checks the endpoint tan dev uses.
  server: { host: "127.0.0.1", port: 5173, strictPort: true },
  build: { outDir: "dist", emptyOutDir: true },
})
