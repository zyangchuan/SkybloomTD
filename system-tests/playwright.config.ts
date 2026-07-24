import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.PLAYWRIGHT_BASE_URL;

export default defineConfig({
  testDir: './specs',
  outputDir: './test-results',
  timeout: 120_000,
  workers: 1,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { outputFolder: './playwright-report', open: 'never' }]],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'SkybloomTD',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
