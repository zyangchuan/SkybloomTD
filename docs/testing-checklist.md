# SkybloomTD — Manual Testing Checklist

**Project:** SkybloomTD  
**Date:** 2026-05-31  
**Tester:** _______________  
**Build / Commit:** _______________  
**Environment:** _______________  

---

## How to Use This Document

Fill in **Actual Result** after running the step. Mark **Status** as `PASS`, `FAIL`, or `SKIP` (skip only if the scenario cannot be reproduced in the current environment). Attach screenshots or logs to any `FAIL` row.

---

## 1. Authentication

### TC-AUTH-01 — Login with Google account

| Field | Detail |
|---|---|
| **Scenario** | User signs in via Google OAuth |
| **Precondition** | User is logged out. Browser has no active session. |
| **Steps** | 1. Navigate to the app root (`/`). <br>2. Click **Google Sign-In**. <br>3. Complete Google OAuth flow. |
| **Expected Result** | User is redirected back to `/auth/callback`, then forwarded to `/dashboard`. Dashboard displays the user's Google name and avatar. |
| **Actual Result** | |
| **Status** | |

---

### TC-AUTH-02 — Successful login redirects to application

| Field | Detail |
|---|---|
| **Scenario** | Post-OAuth redirect lands on the correct page |
| **Precondition** | Google OAuth is completed. |
| **Steps** | 1. Observe the URL after OAuth callback. |
| **Expected Result** | URL is `/dashboard`. No redirect loop or error page. |
| **Actual Result** | |
| **Status** | |

---

### TC-AUTH-03 — Logout works correctly

| Field | Detail |
|---|---|
| **Scenario** | User signs out and session is cleared |
| **Precondition** | User is logged in on the dashboard. |
| **Steps** | 1. Click the **Logout** / sign-out button. <br>2. Observe redirect. <br>3. Try navigating directly to `/dashboard`. |
| **Expected Result** | Session is cleared. User is redirected to the login page. Navigating to `/dashboard` redirects back to login rather than loading the dashboard. |
| **Actual Result** | |
| **Status** | |

---

### TC-AUTH-04 — Unauthorized users cannot access protected pages

| Field | Detail |
|---|---|
| **Scenario** | Unauthenticated access to protected routes is blocked |
| **Precondition** | No active session (use incognito or clear cookies). |
| **Steps** | 1. Directly navigate to `/dashboard`. <br>2. Directly navigate to `/dashboard/games/<any-id>/chapters`. |
| **Expected Result** | Both routes redirect to the login page. No protected data is rendered. |
| **Actual Result** | |
| **Status** | |

---

### TC-AUTH-05 — Session persists after page refresh

| Field | Detail |
|---|---|
| **Scenario** | Refreshing the browser does not log the user out |
| **Precondition** | User is logged in and on the dashboard. |
| **Steps** | 1. Press **F5** / Cmd+R to hard-refresh the page. |
| **Expected Result** | Dashboard reloads with the same user session. No login prompt appears. |
| **Actual Result** | |
| **Status** | |

---

## 2. PDF Upload

The backend accepts multipart upload requests up to 50 MiB, including multipart
form overhead. The browser UI accepts PDF files up to 50 MiB. The reverse proxy
allows request bodies up to 100 MB. Because multipart overhead counts toward the
backend limit, a file exactly 50 MiB may exceed the backend request limit.

### TC-UPLOAD-01 — Upload a valid PDF under 50 MiB

| Field | Detail |
|---|---|
| **Scenario** | Standard successful PDF upload |
| **Precondition** | User is logged in. A PDF file smaller than 50 MiB is available. |
| **Steps** | 1. Click **Upload PDF** on the dashboard. <br>2. Select or drag-and-drop a valid PDF (e.g. 2 MB). <br>3. Enter a game name and click **Upload PDF**. |
| **Expected Result** | Upload succeeds. A new game card appears on the dashboard with status `processing` or `successful`. No error message is shown. |
| **Actual Result** | |
| **Status** | |

---

### TC-UPLOAD-02 — Upload a PDF exactly 50 MiB

| Field | Detail |
|---|---|
| **Scenario** | Boundary value — exactly at the size limit |
| **Precondition** | A multipart request of exactly 52,428,800 bytes (50 × 1024 × 1024), including form overhead, is available. |
| **Steps** | 1. Submit the 50 MiB multipart request to the upload endpoint. <br>2. Verify the request is accepted by the backend. |
| **Expected Result** | File is accepted (no client-side validation error). Upload proceeds normally. |
| **Actual Result** | |
| **Status** | |

---

### TC-UPLOAD-03 — Upload a PDF larger than 50 MiB

| Field | Detail |
|---|---|
| **Scenario** | File exceeds size limit — error must be shown |
| **Precondition** | A PDF file larger than 50 MiB is available. |
| **Steps** | 1. Open the upload modal. <br>2. Select or drop a PDF > 50 MiB. |
| **Expected Result** | Browser validation fires immediately. Error message **"File exceeds 50MB limit."** is displayed in red. Upload button is not enabled. No network request is made. |
| **Actual Result** | |
| **Status** | |

---

### TC-UPLOAD-04 — Upload a non-PDF file

| Field | Detail |
|---|---|
| **Scenario** | Wrong file type is rejected |
| **Precondition** | Non-PDF files are available (e.g. `.docx`, `.png`, `.txt`). |
| **Steps** | 1. Open the upload modal. <br>2. Try selecting or dropping a `.docx` file. <br>3. Repeat with a `.png` file. |
| **Expected Result** | Error message **"Only PDF documents are supported for OCR workflows."** is displayed. The file is not queued. |
| **Actual Result** | |
| **Status** | |

---

### TC-UPLOAD-05 — Uploaded PDF is processed and accessible

| Field | Detail |
|---|---|
| **Scenario** | End-to-end: upload → processing → game available |
| **Precondition** | Valid PDF uploaded (TC-UPLOAD-01 passed). |
| **Steps** | 1. After upload, wait up to 3 minutes for processing to complete. <br>2. Observe game card status changing from `processing` → `successful`. <br>3. Click the game card to open its chapters. |
| **Expected Result** | Game card becomes `is_ready = true`. Chapters page loads with correct chapter titles derived from the uploaded document. |
| **Actual Result** | |
| **Status** | |

---

## 3. Level Access

### TC-LEVEL-01 — All game levels are accessible

| Field | Detail |
|---|---|
| **Scenario** | Every chapter/level listed can be opened |
| **Precondition** | At least one game with multiple chapters exists and `is_ready = true`. |
| **Steps** | 1. Navigate to a game's chapters page. <br>2. Click **Play** on each chapter one by one. |
| **Expected Result** | Every chapter loads the game view without error. No 404 or access-denied page. |
| **Actual Result** | |
| **Status** | |

---

### TC-LEVEL-02 — Users can enter every level without restrictions

| Field | Detail |
|---|---|
| **Scenario** | No gating or prerequisite locks on levels |
| **Precondition** | A game with multiple chapters is available. |
| **Steps** | 1. Without completing chapter 1, directly click **Play** on chapter 2, 3, etc. |
| **Expected Result** | All chapters are playable in any order. No "complete previous level" restriction is shown. |
| **Actual Result** | |
| **Status** | |

---

### TC-LEVEL-03 — Level transitions work correctly

| Field | Detail |
|---|---|
| **Scenario** | Finishing a level progresses to the next correctly |
| **Precondition** | A game session can be completed or ended. |
| **Steps** | 1. Start a chapter and play until the game ends (win or lose). <br>2. Observe the post-game screen or redirect. <br>3. Navigate back to the chapters list. |
| **Expected Result** | Mistakes summary page appears after game ends. Returning to chapters list shows the expected state. No crash or blank screen. |
| **Actual Result** | |
| **Status** | |

---

### TC-LEVEL-04 — Level data loads correctly

| Field | Detail |
|---|---|
| **Scenario** | Game scene receives correct map, enemies, and quiz data for the chapter |
| **Precondition** | A game session is started for a chapter. |
| **Steps** | 1. Open browser DevTools → Network tab. <br>2. Start a chapter. <br>3. Watch WebSocket messages for `game.session.started`. |
| **Expected Result** | `game.session.started` message is received with correct `levelId` and initial game state. Map renders. No console errors about missing data. |
| **Actual Result** | |
| **Status** | |

---

## 4. Connection Recovery

### TC-CONN-01 — Disconnect internet while in-game

| Field | Detail |
|---|---|
| **Scenario** | Application handles network loss gracefully |
| **Precondition** | User is in an active game session. |
| **Steps** | 1. Start a game. <br>2. In OS network settings, disable Wi-Fi / ethernet. <br>3. Observe app behaviour for 10–15 seconds. |
| **Expected Result** | App does not crash. A connection-lost indicator or graceful UI state is shown (or game freezes without throwing unhandled errors). No white screen of death. |
| **Actual Result** | |
| **Status** | |

---

### TC-CONN-02 — Reconnect and verify automatic reconnection

| Field | Detail |
|---|---|
| **Scenario** | WebSocket reconnects without manual page reload |
| **Precondition** | TC-CONN-01 was run and network is still off. |
| **Steps** | 1. Re-enable network after ~15 seconds. <br>2. Observe whether the WebSocket reconnects. |
| **Expected Result** | The game service WebSocket reconnects automatically (or the page prompts to reconnect). Game state is re-synchronised without requiring a full page reload. |
| **Actual Result** | |
| **Status** | |

---

### TC-CONN-03 — User session preserved after reconnection

| Field | Detail |
|---|---|
| **Scenario** | Auth session survives a network interruption |
| **Precondition** | TC-CONN-02 run and network restored. |
| **Steps** | 1. After reconnection, navigate to `/dashboard`. |
| **Expected Result** | User is still logged in. No re-authentication prompt appears. |
| **Actual Result** | |
| **Status** | |

---

### TC-CONN-04 — Application state remains synchronised after reconnection

| Field | Detail |
|---|---|
| **Scenario** | Game state is consistent between client and server after reconnect |
| **Precondition** | Network restored after TC-CONN-01 disconnect. |
| **Steps** | 1. After reconnection, observe the game HUD (gold, HP, wave). <br>2. Compare to state before disconnect. |
| **Expected Result** | HUD values match the last known state. No duplicate enemies or ghost towers. Score / wave counter is correct. |
| **Actual Result** | |
| **Status** | |

---

### TC-CONN-05 — No data loss during temporary disconnection

| Field | Detail |
|---|---|
| **Scenario** | Short disconnect (<5 s) does not corrupt game state |
| **Precondition** | Active game session running. |
| **Steps** | 1. Briefly toggle network off then on (≤5 seconds). <br>2. Resume playing normally. |
| **Expected Result** | Game continues without errors. Tower placements and kills made before disconnect are retained. No duplicate messages sent. |
| **Actual Result** | |
| **Status** | |

---

## 5. Pause / Resume Functionality

### TC-PAUSE-01 — Pause freezes all gameplay activity

| Field | Detail |
|---|---|
| **Scenario** | One button press stops the entire game |
| **Precondition** | Active game session with at least one wave in progress. |
| **Steps** | 1. Click the **Pause** button (top-left). <br>2. Observe the game for 5 seconds. |
| **Expected Result** | Pause window opens. All game activity visually freezes immediately. Server `advanceRuntimeTick` is skipped (confirmed by no incoming `game.state` WebSocket messages while paused). |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-02 — Enemy movement stops

| Field | Detail |
|---|---|
| **Scenario** | Enemies hold their exact positions during pause |
| **Precondition** | Enemies are moving on screen when pause is triggered. |
| **Steps** | 1. Pause the game while enemies are visible on the path. <br>2. Wait 3 seconds. <br>3. Note exact enemy positions before and after wait. |
| **Expected Result** | Enemies do not move one pixel. Lerp interpolation in `update()` is blocked by the `pauseWindowOpen` guard. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-03 — Bird attacks stop

| Field | Detail |
|---|---|
| **Scenario** | Towers (birds) do not fire or deal damage while paused |
| **Precondition** | Towers are placed and attacking when pause is triggered. |
| **Steps** | 1. Pause during an active attack. <br>2. Observe the HP bar of the nearest enemy. |
| **Expected Result** | No attack animations play. Enemy HP does not decrease while paused. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-04 — Cooldown timers stop progressing

| Field | Detail |
|---|---|
| **Scenario** | Server-side attack cooldowns do not advance during pause |
| **Precondition** | Pause immediately after a tower attacks (cooldown just started). |
| **Steps** | 1. Pause right after a tower fires. <br>2. Wait 10 seconds. <br>3. Resume. |
| **Expected Result** | Tower fires again as if only 0 ticks elapsed during pause. The cooldown did not drain while paused. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-05 — Wave progression stops

| Field | Detail |
|---|---|
| **Scenario** | No new enemies spawn and wave timer does not advance during pause |
| **Precondition** | A wave is mid-spawn when paused. |
| **Steps** | 1. Pause during a wave. <br>2. Wait 15 seconds. <br>3. Resume. |
| **Expected Result** | No new enemies appear while paused. The wave continues spawning exactly where it left off immediately after resume. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-06 — Animations stop

| Field | Detail |
|---|---|
| **Scenario** | All sprite-sheet animations freeze on pause |
| **Precondition** | Animated enemies and towers are visible. |
| **Steps** | 1. Pause while animations are playing. <br>2. Observe sprites for 3 seconds. |
| **Expected Result** | All sprite animations freeze on their current frame. Tween animations (e.g. floating quiz prompt) also freeze. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-07 — Extended pause — no game-time passes

| Field | Detail |
|---|---|
| **Scenario** | Long pause does not fast-forward game on resume |
| **Precondition** | Game is in a normal running state. |
| **Steps** | 1. Pause the game. <br>2. Leave it paused for at least 2 minutes. <br>3. Resume. |
| **Expected Result** | Game resumes from the exact paused state. No burst of catch-up ticks. Enemies are at the same positions, HP values unchanged, wave at the same point as when paused. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-08 — Resume restores gameplay from exact paused state

| Field | Detail |
|---|---|
| **Scenario** | All frozen state is correctly restored on resume |
| **Precondition** | Game was paused (TC-PAUSE-01). |
| **Steps** | 1. Note HP, gold, wave number, and enemy positions just before pausing. <br>2. Pause, wait 5 seconds. <br>3. Click **Resume**. |
| **Expected Result** | Game continues from the exact saved state. HUD values match pre-pause values. Animations resume smoothly. No teleportation of enemies or other discontinuities. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-09 — Rapid pause/resume toggling

| Field | Detail |
|---|---|
| **Scenario** | System handles rapid repeated toggling without desync |
| **Precondition** | Active game session running. |
| **Steps** | 1. Click **Pause** then **Resume** 10 times in quick succession (roughly 1 click/second). |
| **Expected Result** | Game state remains consistent after all toggles. No duplicate pause windows. No stuck state where the game appears paused but buttons do not work. |
| **Actual Result** | |
| **Status** | |

---

### TC-PAUSE-10 — No console or server errors during pause/resume

| Field | Detail |
|---|---|
| **Scenario** | Pause/resume cycle is error-free |
| **Precondition** | DevTools Console open. Server logs accessible (`docker logs game-service -f`). |
| **Steps** | 1. Open browser console. <br>2. Pause and resume the game several times. <br>3. Check browser console for errors. <br>4. Check `docker logs game-service` for errors or panics. |
| **Expected Result** | Zero red errors in the browser console. No `panic`, `ERROR`, or unexpected log lines in the server log. |
| **Actual Result** | |
| **Status** | |

---

## 6. General Testing

### TC-GEN-01 — UI elements display correctly

| Field | Detail |
|---|---|
| **Scenario** | All pages render without visual defects |
| **Precondition** | User is logged in. |
| **Steps** | 1. Visit: login page, dashboard, chapters list, game scene, mistakes-summary page. <br>2. Check for overlapping elements, missing images, broken icons, or cut-off text. |
| **Expected Result** | All pages render cleanly at 1440×900. No broken assets or layout overflow. |
| **Actual Result** | |
| **Status** | |

---

### TC-GEN-02 — No browser console errors

| Field | Detail |
|---|---|
| **Scenario** | Normal user journey produces zero JS errors |
| **Precondition** | DevTools Console open with "All levels" filter. |
| **Steps** | 1. Log in. <br>2. Upload a PDF. <br>3. Open a game and play through one wave. <br>4. Pause and resume. <br>5. Open mistakes-summary. |
| **Expected Result** | No red `Error` or `Uncaught` lines in the browser console throughout the journey. Warnings are acceptable but should be noted. |
| **Actual Result** | |
| **Status** | |

---

### TC-GEN-03 — No server-side errors

| Field | Detail |
|---|---|
| **Scenario** | Server logs remain clean during normal use |
| **Precondition** | `docker compose logs -f` running in a terminal. |
| **Steps** | 1. Perform the same journey as TC-GEN-02 while watching container logs. |
| **Expected Result** | No `panic`, `fatal`, `ERROR` (non-debug) in `game-service`, `user-service`, or `document-content` logs. |
| **Actual Result** | |
| **Status** | |

---

### TC-GEN-04 — Application works after page refresh

| Field | Detail |
|---|---|
| **Scenario** | Hard refresh does not break any page |
| **Precondition** | User is logged in. |
| **Steps** | 1. Hard-refresh (`Ctrl+Shift+R`) the dashboard. <br>2. Hard-refresh the chapters list page. <br>3. Hard-refresh the game scene while mid-game. |
| **Expected Result** | Dashboard and chapters pages reload correctly. Game scene on refresh gracefully handles the lost WebSocket state (shows an error or reconnects — no white screen). |
| **Actual Result** | |
| **Status** | |

---

### TC-GEN-05 — Expected behaviour on different screen sizes

| Field | Detail |
|---|---|
| **Scenario** | Core pages are usable across common viewport widths |
| **Precondition** | Use DevTools device emulator. |
| **Steps** | 1. Resize to 1920×1080 (desktop). <br>2. Resize to 1280×800 (laptop). <br>3. Resize to 768×1024 (tablet). <br>4. Resize to 390×844 (mobile, iPhone 14). <br>5. Check login, dashboard, and game scene at each size. |
| **Expected Result** | Login and dashboard pages reflow correctly on all sizes. Game scene may not support mobile — document any known limitations clearly. No critical content is cut off or inaccessible. |
| **Actual Result** | |
| **Status** | |

---

## Summary Table

| ID | Feature | Status | Notes |
|---|---|---|---|
| TC-AUTH-01 | Google login | | |
| TC-AUTH-02 | Post-login redirect | | |
| TC-AUTH-03 | Logout | | |
| TC-AUTH-04 | Unauthorised access blocked | | |
| TC-AUTH-05 | Session persists on refresh | | |
| TC-UPLOAD-01 | Upload valid PDF < 50 MiB | | |
| TC-UPLOAD-02 | Upload PDF exactly 50 MiB | | |
| TC-UPLOAD-03 | Upload PDF > 50 MiB blocked | | |
| TC-UPLOAD-04 | Upload non-PDF blocked | | |
| TC-UPLOAD-05 | PDF processed and accessible | | |
| TC-LEVEL-01 | All levels accessible | | |
| TC-LEVEL-02 | No level gating | | |
| TC-LEVEL-03 | Level transitions | | |
| TC-LEVEL-04 | Level data loads correctly | | |
| TC-CONN-01 | Disconnect gracefully handled | | |
| TC-CONN-02 | Auto-reconnect | | |
| TC-CONN-03 | Session preserved after reconnect | | |
| TC-CONN-04 | State synchronised after reconnect | | |
| TC-CONN-05 | No data loss on short disconnect | | |
| TC-PAUSE-01 | Pause freezes all activity | | |
| TC-PAUSE-02 | Enemy movement stops | | |
| TC-PAUSE-03 | Bird attacks stop | | |
| TC-PAUSE-04 | Cooldown timers stop | | |
| TC-PAUSE-05 | Wave progression stops | | |
| TC-PAUSE-06 | Animations stop | | |
| TC-PAUSE-07 | Extended pause — no time passes | | |
| TC-PAUSE-08 | Resume restores exact state | | |
| TC-PAUSE-09 | Rapid pause/resume toggling | | |
| TC-PAUSE-10 | No console/server errors | | |
| TC-GEN-01 | UI displays correctly | | |
| TC-GEN-02 | No browser console errors | | |
| TC-GEN-03 | No server-side errors | | |
| TC-GEN-04 | Works after page refresh | | |
| TC-GEN-05 | Correct on different screen sizes | | |

---

*Total: 35 test cases*
