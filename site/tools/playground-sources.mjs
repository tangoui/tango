// Reuse the gallery's original package-main demo files through a Go overlay.
// The screenshot command and audit tools are deliberately not linked.
import { mkdirSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';

export function prepareGalleryOverlay(repo, output) {
  mkdirSync(output, { recursive: true });
  const directory = path.join(repo, 'examples/ui-gallery');
  const Replace = {};
  for (const file of readdirSync(directory).sort()) {
    if (!file.endsWith('.go') || file.endsWith('_test.go')) continue;
    if (!file.startsWith('page_') && !['util.go', 'dashboard.go', 'overlays.go', 'overview.go'].includes(file)) continue;
    Replace[path.join(repo, 'site/tools/playground', `gallery_${file}`)] = path.join(directory, file);
  }
  const target = path.join(output, 'gallery-overlay.json');
  writeFileSync(target, JSON.stringify({ Replace }, null, 2));
  return target;
}
