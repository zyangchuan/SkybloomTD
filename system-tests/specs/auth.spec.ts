import { expect, test } from '@playwright/test';
import { ensureDashboard, login } from './helpers/auth';
import { requireCredentials } from './helpers/env';

test.describe('auth and session flow', () => {
  test.beforeEach(() => {
    requireCredentials();
  });

  test('user can sign in and remain signed in after refresh', async ({ page }) => {
    await login(page);
    await page.reload();
    await ensureDashboard(page);
    await expect(page.getByText(/player dashboard/i)).toBeVisible();
  });
});
