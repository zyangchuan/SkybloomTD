'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import OrangeSquare from '@/components/OrangeSquare';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import { syncAuthCookie } from '@/lib/auth-cookie';
import { upsertAuthenticatedUserProfile } from '@/lib/user-profile';

const authRedirectStorageKey = 'skybloom.auth.redirectPath';

function getStoredRedirectPath() {
  const redirectPath = window.sessionStorage.getItem(authRedirectStorageKey);
  window.sessionStorage.removeItem(authRedirectStorageKey);

  if (!redirectPath || !redirectPath.startsWith('/') || redirectPath.startsWith('//')) {
    return '/dashboard';
  }

  return redirectPath;
}

export default function AuthCallbackPage() {
  const router = useRouter();

  useEffect(() => {
    let isMounted = true;

    async function completeSignIn() {
      const supabase = getSupabaseBrowserClient();
      const code = new URLSearchParams(window.location.search).get('code');

      if (code) {
        const { data } = await supabase.auth.exchangeCodeForSession(code);
        await syncAuthCookie(data.session);
        await upsertAuthenticatedUserProfile(data.session).catch((error) => {
          console.error('Failed to store user profile:', error);
        });
      } else {
        const { data } = await supabase.auth.getSession();
        await syncAuthCookie(data.session);
        await upsertAuthenticatedUserProfile(data.session).catch((error) => {
          console.error('Failed to store user profile:', error);
        });
      }

      if (isMounted) {
        router.replace(getStoredRedirectPath());
      }
    }

    completeSignIn().catch(() => {
      if (isMounted) {
        router.replace('/');
      }
    });

    return () => {
      isMounted = false;
    };
  }, [router]);

  return (
    <div className="flex flex-col flex-1 items-center justify-center font-sans">
      <OrangeSquare className="w-full max-w-120 h-80 flex flex-col items-center justify-center gap-4">
        <div className="text-3xl text-white drop-shadow-[0_2px_0_rgba(0,0,0,0.25)] animate-pulse">
          Signing you in...
        </div>
      </OrangeSquare>
    </div>
  );
}
