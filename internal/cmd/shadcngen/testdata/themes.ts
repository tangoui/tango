import { type RegistryItem } from "shadcn/schema"

export const THEMES: RegistryItem[] = [
  {
    name: "neutral",
    title: "Neutral",
    type: "registry:theme",
    cssVars: {
      light: {
        background: "oklch(1 0 0)",
        primary: "oklch(0.205 0 0)",
        radius: "0.625rem",
      },
      dark: {
        background: "oklch(0.145 0 0)",
        border: "oklch(1 0 0 / 10%)",
        "sidebar-primary": "oklch(0.488 0.243 264.376)",
      },
    },
  },
  {
    name: "amber",
    title: "Amber",
    type: "registry:theme",
    cssVars: {
      light: {
        primary: "oklch(0.555 0.163 48.998)",
      },
      dark: {
        primary: "oklch(0.473 0.137 46.201)",
      },
    },
  },
]
