import { expect, test } from '@playwright/test';
import { login } from './helpers/auth';
import { requireCredentials } from './helpers/env';
import { openUploadModal } from './helpers/upload';

test.describe('document upload validation', () => {
  test.beforeEach(() => {
    requireCredentials();
  });

  test('rejects non-PDF files before submission', async ({ page }) => {
    await login(page);
    await openUploadModal(page);
    await page.locator('input[type="file"]').setInputFiles({
      name: 'notes.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('not a pdf'),
    });
    await expect(page.getByText(/only pdf documents are supported/i)).toBeVisible();
  });

  test('keeps submit disabled when required upload fields are missing', async ({ page }) => {
    await login(page);
    await openUploadModal(page);
    await expect(page.getByRole('button', { name: /^upload pdf$/i })).toBeDisabled();
  });
});
