import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { env, requireCredentials, requireReadyLevel } from './helpers/env';

test.describe('level generation and game loading', () => {
  test.beforeEach(() => {
    requireCredentials();
    requireReadyLevel();
  });

  test('user opens a ready sub-chapter and game canvas loads', async ({ page }) => {
    await login(page);
    await page.goto(`/game/?document_id=${env.readyDocumentID}&chapter_id=${env.readyChapterID}&sub_chapter_id=${env.readySubChapterID}`);

    await expect(page.locator('canvas')).toBeVisible({ timeout: 90_000 });
    await expect(page.getByText(/connection lost|level generation failed|failed/i)).toBeHidden();
  });
});
