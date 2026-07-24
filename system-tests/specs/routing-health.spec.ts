import { expect, test } from '@playwright/test';

test.describe('routing and deployment health', () => {
  test('public app and docs routes are reachable', async ({ page, request }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/skybloom/i);

    const docsResponse = await request.get('/docs');
    expect(docsResponse.ok()).toBeTruthy();

    const openAPIResponse = await request.get('/openapi.yaml');
    expect(openAPIResponse.ok()).toBeTruthy();
    await expect(openAPIResponse.text()).resolves.toContain('openapi');
  });

  test('unauthenticated dashboard access returns a login or protected page', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page.getByRole('button', { name: /google sign-in|login to game/i }).or(page.getByText(/player dashboard/i)).first()).toBeVisible();
  });
});
