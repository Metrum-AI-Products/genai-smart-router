import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import {defineConfig} from "vite";

const dashboardRoot = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(dashboardRoot, "..");
const envPath = path.join(repositoryRoot, "env.json");
const sources = new Map([
  ["/data/work-items.ndjson", path.join(repositoryRoot, "work-items.ndjson")],
  ["/data/work-item-events.ndjson", path.join(repositoryRoot, "work-item-events.ndjson")],
]);

function dashboardPort() {
  let values;
  try {
    values = JSON.parse(fs.readFileSync(envPath, "utf8"));
  } catch (error) {
    throw new Error(`Unable to read ${envPath}: ${error.message}`);
  }
  const port = Number(values.WORK_ITEMS_DASHBOARD_PORT);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`WORK_ITEMS_DASHBOARD_PORT must be an integer between 1 and 65535 in ${envPath}`);
  }
  return port;
}

function sourceVersion() {
  return [...sources.values()]
    .map(file => {
      try {
        const stat = fs.statSync(file);
        return `${stat.mtimeMs}:${stat.size}`;
      } catch {
        return "missing";
      }
    })
    .join(":");
}

function sendFile(response, file) {
  try {
    const body = fs.readFileSync(file);
    response.statusCode = 200;
    response.setHeader("Content-Type", "application/x-ndjson; charset=utf-8");
    response.setHeader("Content-Length", body.length);
    response.setHeader("Cache-Control", "no-store");
    response.setHeader("X-Content-Type-Options", "nosniff");
    response.end(body);
  } catch (error) {
    response.statusCode = error.code === "ENOENT" ? 404 : 500;
    response.setHeader("Content-Type", "application/json; charset=utf-8");
    response.end(JSON.stringify({error: "data_source_unavailable"}));
  }
}

function dataMiddleware(request, response, next) {
  const pathname = new URL(request.url, "http://dashboard.local").pathname;
  if (sources.has(pathname)) {
    sendFile(response, sources.get(pathname));
    return;
  }
  if (pathname === "/data/version") {
    const body = JSON.stringify({version: sourceVersion()});
    response.statusCode = 200;
    response.setHeader("Content-Type", "application/json; charset=utf-8");
    response.setHeader("Cache-Control", "no-store");
    response.end(body);
    return;
  }
  next();
}

function workItemDataPlugin() {
  return {
    name: "work-item-data",
    configureServer(server) {
      server.middlewares.use(dataMiddleware);
      for (const file of sources.values()) server.watcher.add(file);
    },
    configurePreviewServer(server) {
      server.middlewares.use(dataMiddleware);
    },
  };
}

const port = dashboardPort();

export default defineConfig({
  plugins: [workItemDataPlugin()],
  server: {host: "0.0.0.0", port, strictPort: true},
  preview: {host: "0.0.0.0", port, strictPort: true},
  build: {chunkSizeWarningLimit: 750},
});
