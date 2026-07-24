import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { env, requireCredentials } from './helpers/env';

test.describe('generated document navigation', () => {
  test.beforeEach(() => {
    requireCredentials();
    test.skip(!env.readyDocumentID, 'Set PLAYWRIGHT_READY_DOCUMENT_ID to run chapter navigation system tests.');
  });

  test('user can open chapters for a ready document', async ({ page }) => {
    await login(page);
    await page.goto(`/dashboard/games/${env.readyDocumentID}/chapters`);
    await expect(page.getByRole('heading', { name: /select chapter/i })).toBeVisible();
    await expect(page.getByText(/level retrieval failed/i)).toBeHidden({ timeout: 30_000 });
  });
});
