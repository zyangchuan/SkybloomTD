import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { requireCredentials } from './helpers/env';
import { gameCard, gameCardPlayButton, uniqueGameName, uploadPdf } from './helpers/upload';

test.describe.serial('document upload, processing, and deletion', () => {
  let uploadedGameName = '';

  test.beforeEach(() => {
    requireCredentials();
  });

  test('user uploads a PDF and sees the game card become playable after refresh', async ({ page }) => {
    test.setTimeout(360_000);

    await login(page);
    uploadedGameName = uniqueGameName();

    await uploadPdf(page, uploadedGameName);
    await expect(gameCard(page, uploadedGameName)).toBeVisible();

    await page.reload();
    const card = gameCard(page, uploadedGameName);
    await expect(card).toBeVisible();
    await expect(gameCardPlayButton(page, uploadedGameName)).toBeVisible({ timeout: 300_000 });
  });

  test('user deletes the uploaded document and it stays removed after refresh', async ({ page }) => {
    test.skip(!uploadedGameName, 'Upload test must run before delete test.');

    await login(page);
    const card = gameCard(page, uploadedGameName);
    await expect(card).toBeVisible({ timeout: 30_000 });

    await card.getByTitle(/delete game and study notes/i).click();
    await card.getByRole('button', { name: /permanently delete/i }).click();

    await expect(gameCard(page, uploadedGameName)).toBeHidden({ timeout: 30_000 });
    await page.reload();
    await expect(gameCard(page, uploadedGameName)).toBeHidden();
  });
});
