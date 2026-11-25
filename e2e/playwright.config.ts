import { defineConfig } from "@playwright/test";
import { mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "..");
const port = 18140;
const scratch = resolve(here, ".tmp");

// A fresh database for every run.
rmSync(scratch, { recursive: true, force: true });
mkdirSync(scratch, { recursive: true });

export default defineConfig({
  testDir: "./tests",
  // The tests share one squadron and run in order.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  reporter: [["list"]],
  timeout: 60_000,
  use: {
    baseURL: `http://localhost:${port}`,
    viewport: { width: 1280, height: 900 },
    trace: "retain-on-failure",
    launchOptions: process.env.CHROMIUM_PATH ? { executablePath: process.env.CHROMIUM_PATH } : {},
  },
  webServer: {
    command: "go run ./cmd/server",
    cwd: repo,
    url: `http://localhost:${port}/health`,
    timeout: 180_000,
    reuseExistingServer: false,
    env: {
      PORT: String(port),
      BASE_URL: `http://localhost:${port}`,
      DB_PATH: resolve(scratch, "e2e.db"),
      SESSION_KEY: "e2e-session-key-e2e-session-key-0123456789",
      COOKIE_SECURE: "false",
      SKIP_EMAIL_VERIFICATION: "true",
      ALLOW_LOG_MAIL: "true",
      ISSUE_EMAIL: "issues@example.org",
    },
  },
});
