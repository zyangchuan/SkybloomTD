import { expect, type Locator, type Page, test } from '@playwright/test';
import { login } from './helpers/auth';
import { env, requireCredentials, requireReadyLevel } from './helpers/env';

const GAME_WIDTH = 2400;
const GAME_HEIGHT = 1600;

test.describe('quiz answer flow', () => {
  test.beforeEach(() => {
    requireCredentials();
    requireReadyLevel();
  });

  test('user can open a quiz and submit an answer', async ({ page }) => {
    test.setTimeout(120_000);

    await login(page);
    await page.goto(`/game/?document_id=${env.readyDocumentID}&chapter_id=${env.readyChapterID}&sub_chapter_id=${env.readySubChapterID}`);

    const canvas = page.locator('canvas');
    await expect(canvas).toBeVisible({ timeout: 90_000 });
    await waitForStableCanvas(page, canvas);

    // Phaser renders these controls on canvas in a 2400x1600 design space.
    await clickCanvasDesignPoint(page, canvas, 250, 770);
    await waitForQuizWindow(page, canvas);

    const quizFrame = page.frameLocator('#quiz-iframe');
    const firstAnswer = quizFrame.locator('button.option-9slice').first();
    await expect(firstAnswer).toBeVisible();

    const start = await page.evaluate(() => performance.now());
    await firstAnswer.click();

    await expect(firstAnswer).toHaveClass(/highlight-(correct|incorrect)/, {
      timeout: env.quizFeedbackLatencyMs,
    });
    const end = await page.evaluate(() => performance.now());
    const feedbackLatencyMs = end - start;

    expect(feedbackLatencyMs).toBeLessThanOrEqual(env.quizFeedbackLatencyMs);
    await expect(page.locator('#quiz-iframe')).toBeHidden({ timeout: 15_000 });
    await expect(page.getByText(/quiz not found|no quizzes remaining|failed to load quiz/i)).toBeHidden();
  });
});

async function clickCanvasDesignPoint(
  page: Page,
  canvas: Locator,
  designX: number,
  designY: number,
) {
  const box = await canvas.boundingBox();
  if (!box) {
    throw new Error('Game canvas bounding box was not available.');
  }
  const x = box.x + (designX / GAME_WIDTH) * box.width;
  const y = box.y + (designY / GAME_HEIGHT) * box.height;
  await page.mouse.click(x, y);
}

async function waitForStableCanvas(page: Page, canvas: Locator) {
  let previousBox = await canvas.boundingBox();
  if (!previousBox) {
    throw new Error('Game canvas bounding box was not available.');
  }

  for (let attempt = 0; attempt < 10; attempt += 1) {
    await page.waitForTimeout(250);
    const currentBox = await canvas.boundingBox();
    if (!currentBox) {
      throw new Error('Game canvas bounding box was not available.');
    }

    const stable =
      Math.abs(currentBox.width - previousBox.width) < 1 &&
      Math.abs(currentBox.height - previousBox.height) < 1 &&
      currentBox.width > 0 &&
      currentBox.height > 0;

    if (stable) return;
    previousBox = currentBox;
  }
}

async function waitForQuizWindow(page: Page, canvas: Locator) {
  const quizFrame = page.locator('#quiz-iframe');

  await expect
    .poll(
      async () => {
        if (await quizFrame.isVisible().catch(() => false)) {
          return true;
        }

        await clickCanvasDesignPoint(page, canvas, 2270, 770);
        await page.waitForTimeout(750);

        return quizFrame.isVisible().catch(() => false);
      },
      {
        timeout: 45_000,
        message: 'Bird of Wisdom did not open the quiz window after the game started.',
      },
    )
    .toBe(true);
}
