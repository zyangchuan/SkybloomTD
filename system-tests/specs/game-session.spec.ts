import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { env, requireCredentials, requireReadyLevel } from './helpers/env';

test.describe('game session smoke flow', () => {
  test.beforeEach(() => {
    requireCredentials();
    requireReadyLevel();
  });

  test('game page establishes websocket-driven session UI', async ({ page }) => {
    await login(page);

    const websocketPromise = page.waitForEvent('websocket', { timeout: 30_000 });
    await page.goto(`/game/?document_id=${env.readyDocumentID}&chapter_id=${env.readyChapterID}&sub_chapter_id=${env.readySubChapterID}`);

    const ws = await websocketPromise;
    expect(ws.url()).toContain('/api/game-service/ws');
    await expect(page.locator('canvas')).toBeVisible({ timeout: 90_000 });
  });
});
