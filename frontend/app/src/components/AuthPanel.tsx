'use client';

import type { Session } from '@supabase/supabase-js';
import Image from 'next/image';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import ButtonWhite from '@/components/ButtonWhite';
import ButtonGreen from '@/components/ButtonGreen';
import RetroInput from '@/components/RetroInput';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import { clearAuthCookie, syncAuthCookie } from '@/lib/auth-cookie';
import { upsertAuthenticatedUserProfile } from '@/lib/user-profile';

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

const authRedirectStorageKey = 'skybloom.auth.redirectPath';
const sessionCheckTimeoutMs = 8000;

function getCurrentPath() {
  return `${window.location.pathname}${window.location.search}`;
}

function rememberAuthRedirectPath() {
  window.sessionStorage.setItem(authRedirectStorageKey, getCurrentPath());
}

function getAuthCallbackURL() {
  const envUrl = process.env.NEXT_PUBLIC_SUPABASE_REDIRECT_URL || process.env.SUPABASE_REDIRECT_URL;
  if (envUrl) {
    return envUrl;
  }
  return `${window.location.origin}/auth/callback`;
}

function getAuthErrorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : 'Unable to check your session. Please try signing in again.';
}

async function withSessionCheckTimeout<T>(promise: Promise<T>) {
  let timeoutId: number | undefined;

  try {
    return await Promise.race([
      promise,
      new Promise<never>((_, reject) => {
        timeoutId = window.setTimeout(() => {
          reject(new Error('Session check timed out. You can still sign in.'));
        }, sessionCheckTimeoutMs);
      }),
    ]);
  } finally {
    if (timeoutId) {
      window.clearTimeout(timeoutId);
    }
  }
}

async function syncSessionAndProfile(session: Session | null) {
  await syncAuthCookie(session);
  await upsertAuthenticatedUserProfile(session).catch((error) => {
    console.error('Failed to store user profile:', error);
  });
}

export default function AuthPanel() {
  const router = useRouter();
  const [session, setSession] = useState<Session | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [authError, setAuthError] = useState<string | null>(null);
  const [signUpSuccess, setSignUpSuccess] = useState(false);

  // Form State
  const [authMode, setAuthMode] = useState<'signin' | 'signup'>('signin');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  useEffect(() => {
    let isMounted = true;
    let unsubscribeAuth: (() => void) | undefined;
    const loadingFallbackId = window.setTimeout(() => {
      if (isMounted) {
        setIsLoading(false);
      }
    }, sessionCheckTimeoutMs);

    async function checkSession() {
      try {
        const supabase = getSupabaseBrowserClient();

        const {
          data: { subscription },
        } = supabase.auth.onAuthStateChange(async (_event, nextSession) => {
          if (!isMounted) return;
          setSession(nextSession);
          setIsLoading(false);
          setAuthError(null);
          setIsSubmitting(false);
          await syncSessionAndProfile(nextSession).catch((error) => {
            console.error('Failed to sync auth session:', error);
          });
          if (nextSession) {
            router.push('/dashboard');
          }
        });

        unsubscribeAuth = () => subscription.unsubscribe();

        const { data, error } = await withSessionCheckTimeout(supabase.auth.getSession());
        if (!isMounted) {
          return;
        }

        if (error) {
          setAuthError(error.message);
        }

        window.clearTimeout(loadingFallbackId);
        setSession(data.session);
        setIsLoading(false);
        if (data.session) {
          await syncSessionAndProfile(data.session).catch((error) => {
            console.error('Failed to sync auth session:', error);
          });
          router.push('/dashboard');
        }
      } catch (error) {
        if (!isMounted) {
          return;
        }

        window.clearTimeout(loadingFallbackId);
        setSession(null);
        setIsLoading(false);
        setAuthError(getAuthErrorMessage(error));
      }
    }

    checkSession();

    return () => {
      isMounted = false;
      unsubscribeAuth?.();
    };
  }, [router]);

  async function handleCredentialsSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!email || !password) {
      setAuthError('Please fill in all fields.');
      return;
    }

    if (authMode === 'signup' && password !== confirmPassword) {
      setAuthError('Passwords do not match.');
      return;
    }

    const supabase = getSupabaseBrowserClient();
    setIsSubmitting(true);
    setAuthError(null);
    setSignUpSuccess(false);
    rememberAuthRedirectPath();

    if (authMode === 'signin') {
      const { data, error } = await supabase.auth.signInWithPassword({
        email,
        password,
      });

      if (error) {
        setAuthError(error.message);
        setIsSubmitting(false);
      } else if (data.session) {
        await syncSessionAndProfile(data.session).catch((error) => {
          console.error('Failed to sync auth session:', error);
        });
      }
    } else {
      const { data, error } = await supabase.auth.signUp({
        email,
        password,
        options: {
          emailRedirectTo: getAuthCallbackURL(),
        },
      });

      if (error) {
        setAuthError(error.message);
        setIsSubmitting(false);
      } else {
        setIsSubmitting(false);
        if (data.user && data.session) {
          // Logged in immediately
          router.push('/dashboard');
        } else {
          setSignUpSuccess(true);
          setEmail('');
          setPassword('');
          setConfirmPassword('');
        }
      }
    }
  }

  async function signInWithGoogle() {
    const supabase = getSupabaseBrowserClient();

    setIsSubmitting(true);
    setAuthError(null);
    setSignUpSuccess(false);
    rememberAuthRedirectPath();

    const { error } = await supabase.auth.signInWithOAuth({
      provider: 'google',
      options: {
        redirectTo: getAuthCallbackURL(),
      },
    });

    if (error) {
      setAuthError(error.message);
      setIsSubmitting(false);
    }
  }

  async function signOut() {
    const supabase = getSupabaseBrowserClient();

    setIsSubmitting(true);
    setAuthError(null);

    const { error } = await supabase.auth.signOut();
    await clearAuthCookie();

    if (error) {
      setAuthError(error.message);
    }

    setIsSubmitting(false);
  }

  if (session) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center gap-5 px-8 text-center">
        <div className="text-3xl text-white drop-shadow-[0_2px_0_rgba(0,0,0,0.25)]">
          Welcome, {getDisplayName(session)}
        </div>
        <div className="text-xl text-white opacity-90 animate-pulse">
          Redirecting to dashboard...
        </div>
        <ButtonWhite
          className="h-10 w-52 text-xl"
          disabled={isSubmitting}
          onClick={signOut}
        >
          Sign out
        </ButtonWhite>
        {authError ? <p className="max-w-72 text-sm text-red-900">{authError}</p> : null}
      </div>
    );
  }

  return (
    <div className="flex h-full w-full flex-col items-center justify-center px-6 text-center select-none py-4">
      {/* Social Logins at the top */}
      <ButtonWhite
        className="h-10 w-full text-lg cursor-pointer"
        disabled={isLoading || isSubmitting}
        onClick={signInWithGoogle}
      >
        <Image src="/google.png" alt="" width={18} height={18} />
        <span className="ml-2 -mt-0.5">
          Google Sign-In
        </span>
      </ButtonWhite>

      {/* Separation Divider */}
      <div className="flex items-center justify-center gap-3 my-2 w-full select-none">
        <span className="h-[2.5px] bg-[#4a1900]/20 flex-1" />
        <span className="text-[#4a1900]/80 text-sm font-bold tracking-widest font-mono select-none drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
          OR
        </span>
        <span className="h-[2.5px] bg-[#4a1900]/20 flex-1" />
      </div>

      {/* Tab Switchers */}
      <div className="flex w-full gap-2 mb-3">
        <button
          onClick={() => {
            setAuthMode('signin');
            setAuthError(null);
            setSignUpSuccess(false);
          }}
          className={`flex-1 py-1 text-xl cursor-pointer transition-all ${
            authMode === 'signin'
              ? 'text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] border-b-4 border-yellow-300 font-bold scale-105'
              : 'text-[#4a1900]/60 hover:text-[#4a1900] border-b-2 border-[#4a1900]/20 font-bold'
          }`}
        >
          Sign In
        </button>
        <button
          onClick={() => {
            setAuthMode('signup');
            setAuthError(null);
            setSignUpSuccess(false);
          }}
          className={`flex-1 py-1 text-xl cursor-pointer transition-all ${
            authMode === 'signup'
              ? 'text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] border-b-4 border-yellow-300 font-bold scale-105'
              : 'text-[#4a1900]/60 hover:text-[#4a1900] border-b-2 border-[#4a1900]/20 font-bold'
          }`}
        >
          Sign Up
        </button>
      </div>

      {signUpSuccess ? (
        <div className="flex flex-col items-center justify-center gap-4 py-6">
          <div className="text-2xl text-yellow-300 drop-shadow-[0_1px_0_rgba(0,0,0,0.25)] font-bold">
            Success!
          </div>
          <p className="text-[#4a1900] text-md max-w-72 leading-relaxed font-bold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
            Please check your email to verify your account and complete sign up!
          </p>
          <ButtonGreen
            className="h-10 w-44 text-xl mt-2 text-white font-bold drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
            onClick={() => setSignUpSuccess(false)}
          >
            Back to Login
          </ButtonGreen>
        </div>
      ) : (
        <form onSubmit={handleCredentialsSubmit} className="flex flex-col w-full gap-2.5">
          <RetroInput
            id="email-input"
            label="Email"
            type="email"
            placeholder="enter your email..."
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            disabled={isLoading || isSubmitting}
            required
          />

          <RetroInput
            id="password-input"
            label="Password"
            type="password"
            placeholder="enter your password..."
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={isLoading || isSubmitting}
            required
          />

          {authMode === 'signup' && (
            <RetroInput
              id="confirm-password-input"
              label="Confirm Password"
              type="password"
              placeholder="confirm your password..."
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              disabled={isLoading || isSubmitting}
              required
            />
          )}

          <ButtonGreen
            className="h-11 w-full text-xl mt-1 text-white font-bold drop-shadow-[0_1.5px_0_rgba(0,0,0,0.35)] hover:brightness-105"
            disabled={isLoading || isSubmitting}
            type="submit"
          >
            {isSubmitting
              ? 'Processing...'
              : authMode === 'signin'
              ? 'Login to Game'
              : 'Create Account'}
          </ButtonGreen>

          {isLoading ? (
            <p className="text-sm text-[#4a1900]/70 animate-pulse mt-0.5 font-bold">Checking session...</p>
          ) : null}
          {authError ? (
            <p className="max-w-72 text-sm text-red-900 bg-red-100/90 border border-red-400 rounded-md py-1 px-3 mt-0.5 text-center font-mono mx-auto">
              {authError}
            </p>
          ) : null}
        </form>
      )}
    </div>
  );
}
