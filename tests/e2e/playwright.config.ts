import { defineConfig, devices } from '@playwright/test';

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required for Playwright tests`);
  return value;
}

const serverEnv = Object.fromEntries(
  Object.entries(process.env).filter((entry): entry is [string, string] => typeof entry[1] === 'string'),
);
const adminPassword = requiredEnv('E2E_ADMIN_PASSWORD');
const sessionSecret = requiredEnv('E2E_SESSION_SECRET');

export default defineConfig({
  testDir: '.',
  timeout: 45_000,
  expect: { timeout: 8_000 },
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  outputDir: 'test-results',
  use: {
    baseURL: 'http://127.0.0.1:18080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    { name: 'Desktop Chromium 1440', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } },
    { name: 'Desktop Firefox 1440', use: { ...devices['Desktop Firefox'], viewport: { width: 1440, height: 900 } } },
    { name: 'Tablet WebKit 768', use: { ...devices['iPad Mini'] } },
    { name: 'Mobile Chromium', use: { ...devices['Pixel 7'] } },
    { name: 'Mobile WebKit', use: { ...devices['iPhone 13'] } },
  ],
  webServer: {
    command: 'cd ../.. && go run ./cmd/docshub',
    env: {
      ...serverEnv,
      ADMIN_PASSWORD: adminPassword,
      SESSION_SECRET: sessionSecret,
      DATA_DIR: process.env.E2E_DATA_DIR || `/tmp/docshub-playwright-${process.pid}`,
      RATE_LIMIT_ENABLED: 'false',
      ADDR: '127.0.0.1:18080',
    },
    url: 'http://127.0.0.1:18080/healthz',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
