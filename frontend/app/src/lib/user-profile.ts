import type { Session } from '@supabase/supabase-js';

function firstNonEmpty(...values: unknown[]) {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) {
      return value.trim();
    }
  }

  return '';
}

function getUserName(session: Session) {
  const metadata = session.user.user_metadata ?? {};
  const displayName = firstNonEmpty(metadata.full_name, metadata.name, metadata.user_name);

  if (displayName) {
    return displayName;
  }

  const email = session.user.email?.trim();
  if (email) {
    return email.split('@')[0] || email;
  }

  return 'Player';
}

export async function upsertAuthenticatedUserProfile(session: Session | null) {
  if (!session) {
    return;
  }

  const response = await fetch('/api/users/me', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      email: session.user.email,
      user_name: getUserName(session),
      metadata: session.user.user_metadata ?? {},
    }),
  });

  if (!response.ok) {
    const message = await response.text().catch(() => '');
    throw new Error(message || 'Failed to store user profile.');
  }
}
