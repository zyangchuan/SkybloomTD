import { expect, type Locator, type Page } from '@playwright/test';
import path from 'node:path';
export const testPdfPath = process.env.PLAYWRIGHT_TEST_PDF_PATH || path.resolve(process.cwd(), 'testpdf.pdf');

export function uniqueGameName(prefix = 'System Test Game') {
  return `${prefix} ${Date.now()}`;
}

export async function openUploadModal(page: Page) {
  await page.getByRole('button', { name: /upload document/i }).click();
  await expect(page.getByRole('heading', { name: /upload document/i })).toBeVisible();
}

export function gameCard(page: Page, gameName: string): Locator {
  return page
    .getByText(gameName, { exact: true })
    .locator('xpath=ancestor::div[.//button[@title="Delete game and study notes"]][1]');
}

export function gameCardPlayButton(page: Page, gameName: string): Locator {
  return gameCard(page, gameName).getByRole('button', { name: /play/i });
}

export async function uploadPdf(page: Page, gameName: string) {
  await openUploadModal(page);
  await page.getByLabel(/game name/i).fill(gameName);
  await page.locator('input[type="file"]').setInputFiles(testPdfPath);
  await page.getByRole('button', { name: /^upload pdf$/i }).click();
  await expect(page.getByText(/document uploaded/i)).toBeVisible({ timeout: 60_000 });
  await page.getByRole('button', { name: /return to dashboard/i }).click();
  await expect(gameCard(page, gameName)).toBeVisible();
}
