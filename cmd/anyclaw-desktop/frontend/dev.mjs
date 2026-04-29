import { createServer } from "node:http";
import { watch } from "node:fs";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const srcFile = path.join(__dirname, "src", "index.html");
const port = 34115;

let html = await readFile(srcFile, "utf8");

watch(srcFile, { persistent: true }, async () => {
  html = await readFile(srcFile, "utf8");
  console.log("Reloaded desktop shell source");
});

const server = createServer((req, res) => {
  if (req.url && req.url !== "/" && req.url !== "/index.html") {
    res.writeHead(404);
    res.end("Not found");
    return;
  }

  res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
  res.end(html);
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Desktop shell dev server listening on http://127.0.0.1:${port}`);
});
