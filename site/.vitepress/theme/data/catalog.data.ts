// Component counts for the homepage stat strip, read from the catalog that
// tools/build-playground.mjs generates from ui/AUTHORING.md §14 (npm run
// docs:gen). Without it the strip omits the numbers instead of guessing.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const file = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../generated/components.json");

export interface CatalogStats {
  entries: number | null; // rows of the component table
  names: number | null; // distinct public component entry points
  groups: string[];
}

export default {
  watch: [file],
  load(): CatalogStats {
    if (!fs.existsSync(file)) return { entries: null, names: null, groups: [] };
    const catalog = JSON.parse(fs.readFileSync(file, "utf8"));
    const names = new Set<string>();
    for (const c of catalog.components) for (const n of c.names) names.add(n);
    return { entries: catalog.components.length, names: names.size, groups: catalog.groups };
  },
};
