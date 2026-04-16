import { copyFile, mkdir } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const srcDir = path.join(__dirname, "src");
const distDir = path.join(__dirname, "dist");

await mkdir(distDir, { recursive: true });
await copyFile(path.join(srcDir, "index.html"), path.join(distDir, "index.html"));

console.log(`Desktop shell built into ${distDir}`);
