import { cookies } from 'next/headers';
import { NextResponse } from 'next/server';

const cookieName = process.env.AUTH_ACCESS_TOKEN_COOKIE || 'skybloom_access_token';
const maxAgeSeconds = 60 * 60;
const cookieSecure = parseBoolean(process.env.AUTH_COOKIE_SECURE, false);

function parseBoolean(value: string | undefined, fallback: boolean) {
  if (!value) return fallback;
  return ['1', 'true', 'yes', 'on'].includes(value.trim().toLowerCase());
}

export async function POST(request: Request) {
  const { access_token } = await request.json().catch(() => ({ access_token: '' }));

  if (typeof access_token !== 'string' || !access_token.trim()) {
    return NextResponse.json({ error: 'access_token is required' }, { status: 400 });
  }

  const cookieStore = await cookies();
  cookieStore.set(cookieName, access_token.trim(), {
    httpOnly: true,
    secure: cookieSecure,
    sameSite: 'lax',
    path: '/',
    maxAge: maxAgeSeconds,
  });

  return NextResponse.json({ status: 'ok' });
}

// delete means
export async function DELETE() {
  const cookieStore = await cookies();
  cookieStore.set(cookieName, '', {
    httpOnly: true,
    secure: cookieSecure,
    sameSite: 'lax',
    path: '/',
    maxAge: 0,
  });

  return NextResponse.json({ status: 'ok' });
}
