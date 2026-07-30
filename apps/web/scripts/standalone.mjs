import { cp, mkdir } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const webRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const standaloneRoot = join(webRoot, ".next", "standalone");
const command = process.argv[2];

if (command === "prepare") {
  await mkdir(join(standaloneRoot, ".next"), { recursive: true });
  await cp(join(webRoot, ".next", "static"), join(standaloneRoot, ".next", "static"), { recursive: true, force: true });
  await cp(join(webRoot, "public"), join(standaloneRoot, "public"), { recursive: true, force: true });
} else if (command === "start") {
  process.env.HOSTNAME ??= "0.0.0.0";
  process.env.PORT ??= "8080";
  await import(pathToFileURL(join(standaloneRoot, "server.js")).href);
} else {
  throw new Error("usage: standalone.mjs prepare|start");
}