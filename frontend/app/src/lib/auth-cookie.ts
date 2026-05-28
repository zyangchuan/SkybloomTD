import type { Session } from '@supabase/supabase-js';

export async function syncAuthCookie(session: Session | null) {
  if (!session?.access_token) {
    await clearAuthCookie();
    return;
  }

  await fetch('/api/auth/session-cookie', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ access_token: session.access_token }),
  });
}

export async function clearAuthCookie() {
  await fetch('/api/auth/session-cookie', {
    method: 'DELETE',
  });
}
