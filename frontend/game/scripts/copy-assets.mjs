import { cpSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const rootDir = fileURLToPath(new URL('..', import.meta.url));
const sourceDir = join(rootDir, 'assets');
const targetDir = join(rootDir, 'dist', 'assets');

if (!existsSync(sourceDir)) {
  throw new Error(`Missing game asset directory: ${sourceDir}`);
}

mkdirSync(targetDir, { recursive: true });
cpSync(sourceDir, targetDir, {
  recursive: true,
  force: true,
  filter: (source) => !source.endsWith('.DS_Store'),
});

console.log(`Copied game assets to ${targetDir}`);
