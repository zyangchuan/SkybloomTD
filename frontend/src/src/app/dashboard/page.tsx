'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Image from 'next/image';
import type { Session } from '@supabase/supabase-js';
import OrangeSquare from '@/components/OrangeSquare';
import BlueSquare from '@/components/BlueSquare';
import ButtonOrange from '@/components/ButtonOrange';
import { getSupabaseBrowserClient } from '@/lib/supabase';

function getDisplayName(session: Session) {
  const fullName = session.user.user_metadata?.full_name;
  const name = session.user.user_metadata?.name;

  if (typeof fullName === 'string' && fullName.trim()) {
    return fullName;
  }

  if (typeof name === 'string' && name.trim()) {
    return name;
  }

  return session.user.email ?? 'Player';
}

function getAvatarUrl(session: Session) {
  const avatarUrl = session.user.user_metadata?.avatar_url;
  const picture = session.user.user_metadata?.picture;

  if (typeof avatarUrl === 'string' && avatarUrl.trim()) {
    return avatarUrl;
  }

  if (typeof picture === 'string' && picture.trim()) {
    return picture;
  }

  return null;
}

export default function DashboardPage() {
  const router = useRouter();
  const [session, setSession] = useState<Session | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);

  useEffect(() => {
    let isMounted = true;
    const supabase = getSupabaseBrowserClient();

    supabase.auth.getSession().then(({ data, error }) => {
      if (!isMounted) return;

      if (error) {
        setAuthError(error.message);
      }

      if (data.session) {
        setSession(data.session);
        setIsLoading(false);
      } else {
        // No session found, bounce to login
        router.push('/');
      }
    });

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      if (!isMounted) return;

      setSession(nextSession);
      setIsLoading(false);
      if (!nextSession) {
        router.push('/');
      }
    });

    return () => {
      isMounted = false;
      subscription.unsubscribe();
    };
  }, [router]);

  async function handleSignOut() {
    const supabase = getSupabaseBrowserClient();
    setIsSubmitting(true);
    setAuthError(null);

    const { error } = await supabase.auth.signOut();

    if (error) {
      setAuthError(error.message);
      setIsSubmitting(false);
    } else {
      router.push('/');
    }
  }

  if (isLoading) {
    return (
      <div className="flex flex-col flex-1 items-center justify-center font-sans">
        <OrangeSquare className="w-full max-w-120 h-80 flex flex-col items-center justify-center gap-4">
          <div className="text-3xl text-white drop-shadow-[0_2px_0_rgba(0,0,0,0.25)] animate-pulse">
            Loading player profile...
          </div>
        </OrangeSquare>
      </div>
    );
  }

  if (!session) {
    return null; // Will redirect shortly via useEffect
  }

  const displayName = getDisplayName(session);
  const email = session.user.email;
  const avatarUrl = getAvatarUrl(session);

  return (
    <div className="flex flex-col flex-1 items-center justify-center font-sans p-4">
      <Image src="/skybloomtd-logo.png" alt="SkybloomTD" width={400} height={400} className="mb-6 drop-shadow-lg" />
      
      <OrangeSquare className="w-full max-w-140 flex flex-col items-center justify-center py-10 px-8">
        <h1 className="text-4xl text-white mb-6 drop-shadow-[0_2px_0_rgba(0,0,0,0.25)] tracking-wider">
          Player Dashboard
        </h1>

        <BlueSquare className="w-full flex flex-col items-center justify-center py-6 px-4 mb-6">
          <div className="flex flex-col items-center gap-4 w-full">
            {avatarUrl ? (
              <div className="relative w-24 h-24 rounded-full overflow-hidden border-4 border-white shadow-md">
                <Image 
                  src={avatarUrl} 
                  alt={displayName} 
                  fill 
                  sizes="96px"
                  priority
                  className="object-cover"
                />
              </div>
            ) : (
              <div className="w-24 h-24 rounded-full bg-orange-600 border-4 border-white shadow-md flex items-center justify-center text-white text-4xl">
                {displayName.charAt(0).toUpperCase()}
              </div>
            )}

            <div className="text-center">
              <h2 className="text-3xl text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]">
                {displayName}
              </h2>
              <p className="text-md text-white font-mono mt-1 drop-shadow-[0_1px_0_rgba(0,0,0,0.25)]">
                {email}
              </p>
            </div>
          </div>
        </BlueSquare>

        <div className="flex flex-col w-full gap-4 items-center">
          <ButtonOrange
            className="h-12 w-64 text-2xl text-white drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] hover:brightness-110 active:scale-95 transition-all duration-100"
            disabled={isSubmitting}
            onClick={handleSignOut}
          >
            {isSubmitting ? 'Signing Out...' : 'Sign Out'}
          </ButtonOrange>

          {authError && (
            <p className="text-red-900 bg-red-100 border border-red-400 rounded-lg py-2 px-4 mt-2 text-sm max-w-xs text-center font-mono">
              Error: {authError}
            </p>
          )}
        </div>
      </OrangeSquare>
    </div>
  );
}

