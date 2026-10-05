import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:19427",
    channel: "chrome",
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
  },
  webServer: {
    command: "../bin/ai-atlas-server",
    url: "http://127.0.0.1:19427",
    reuseExistingServer: false,
    timeout: 30000,
    env: {
      WAILS_SERVER_HOST: "127.0.0.1",
      WAILS_SERVER_PORT: "19427",
      CODEX_HOME: process.cwd() + "/../.test-data/home",
      AI_ATLAS_DB: process.cwd() + "/../.test-data/atlas.sqlite",
    },
  },
});
