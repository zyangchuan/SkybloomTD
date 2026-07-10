import { test } from '@playwright/test';

export const env = {
  baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://localhost',
  email: process.env.PLAYWRIGHT_TEST_EMAIL || '',
  password: process.env.PLAYWRIGHT_TEST_PASSWORD || '',
  readyDocumentID: process.env.PLAYWRIGHT_READY_DOCUMENT_ID || '',
  readyChapterID: process.env.PLAYWRIGHT_READY_CHAPTER_ID || '',
  readySubChapterID: process.env.PLAYWRIGHT_READY_SUB_CHAPTER_ID || '',
  quizFeedbackLatencyMs: Number(process.env.PLAYWRIGHT_QUIZ_FEEDBACK_LATENCY_MS || 3000),
};

export function requireCredentials() {
  test.skip(!env.email || !env.password, 'Set PLAYWRIGHT_TEST_EMAIL and PLAYWRIGHT_TEST_PASSWORD to run this system test.');
}

export function requireReadyLevel() {
  test.skip(
    !env.readyDocumentID || !env.readyChapterID || !env.readySubChapterID,
    'Set PLAYWRIGHT_READY_DOCUMENT_ID, PLAYWRIGHT_READY_CHAPTER_ID, and PLAYWRIGHT_READY_SUB_CHAPTER_ID to run this game system test.',
  );
}
