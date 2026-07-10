import { expect, type Page } from '@playwright/test';
import { env } from './env';

export async function login(page: Page) {
  await page.goto('/');
  await page.getByLabel('Email').fill(env.email);
  await page.getByLabel('Password').fill(env.password);
  await page.getByRole('button', { name: /login to game/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 30_000 });
  await expect(page.getByText(/player dashboard/i)).toBeVisible();
}

export async function ensureDashboard(page: Page) {
  if (!/\/dashboard/.test(page.url())) {
    await page.goto('/dashboard');
  }
  await expect(page.getByText(/player dashboard/i)).toBeVisible();
}
