import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { env, requireCredentials, requireReadyLevel } from './helpers/env';

test.describe('game reconnect/session restore', () => {
  test.beforeEach(() => {
    requireCredentials();
    requireReadyLevel();
  });

  test('refreshing the game page restores a visible game scene', async ({ page }) => {
    await login(page);
    await page.goto(`/game/?document_id=${env.readyDocumentID}&chapter_id=${env.readyChapterID}&sub_chapter_id=${env.readySubChapterID}`);
    await expect(page.locator('canvas')).toBeVisible({ timeout: 90_000 });

    await page.reload();
    await expect(page.locator('canvas')).toBeVisible({ timeout: 90_000 });
    await expect(page.getByText(/connection lost/i)).toBeHidden();
  });
});
