'use client';

import { useEffect, useState, useCallback } from 'react';
import { 
  Gamepad2, 
  RefreshCw,
  Loader2,
  AlertTriangle,
  Library
} from 'lucide-react';
import { useRouter } from 'next/navigation';
import Image from 'next/image';
import type { Session } from '@supabase/supabase-js';
import axios from 'axios';
import OrangeSquare from '@/components/OrangeSquare';
import BlueSquare from '@/components/BlueSquare';
import ButtonOrange from '@/components/ButtonOrange';
import ButtonGreen from '@/components/ButtonGreen';
import GameCardItem from '@/components/GameCardItem';
import LibraryGameCard, { GameLibrarySummary } from '@/components/LibraryGameCard';
import UploadDocumentModal from '@/components/UploadDocumentModal';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import { clearAuthCookie, syncAuthCookie } from '@/lib/auth-cookie';
import { upsertAuthenticatedUserProfile } from '@/lib/user-profile';
import tabStyles from '@/components/MainTabs.module.css';

interface DocumentSummary {
  document_id: string;
  filename: string;
  game_name: string;
  is_ready: boolean;
  is_public: boolean;
  task_id: string;
  created_at: string;
  updated_at: string;
}

interface TaskStatusResponse {
  task_id: string;
  document_id: string;
  status: 'queued' | 'processing' | 'successful' | 'failed';
  error: string | null;
  updated_at: string;
}

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
  const [isUploadOpen, setIsUploadOpen] = useState(false);

  // Main Tab State
  const [activeMainTab, setActiveMainTab] = useState<'dashboard' | 'library'>('dashboard');

  // Tab State
  const [activeTab, setActiveTab] = useState<'my-games' | 'starred-games'>('my-games');

  // Document history and status states
  const [documents, setDocuments] = useState<DocumentSummary[]>([]);
  const [taskStatuses, setTaskStatuses] = useState<Record<string, TaskStatusResponse>>({});
  const [isDocsLoading, setIsDocsLoading] = useState(false);
  const [docsError, setDocsError] = useState<string | null>(null);

  // Starred Games states
  const [starredGames, setStarredGames] = useState<GameLibrarySummary[]>([]);
  const [isStarredLoading, setIsStarredLoading] = useState(false);
  const [starredError, setStarredError] = useState<string | null>(null);
  const [starredCursor, setStarredCursor] = useState<string | null>(null);
  const [starredHasMore, setStarredHasMore] = useState(false);

  // Public Library states
  const [publicGames, setPublicGames] = useState<GameLibrarySummary[]>([]);
  const [isPublicLoading, setIsPublicLoading] = useState(false);
  const [publicError, setPublicError] = useState<string | null>(null);
  const [publicCursor, setPublicCursor] = useState<string | null>(null);
  const [publicHasMore, setPublicHasMore] = useState(false);

  // Deleting document states
  const [deletingDocId, setDeletingDocId] = useState<string | null>(null);
  const [isDeletingInProgress, setIsDeletingInProgress] = useState(false);

  // Fetch individual document task status
  const fetchTaskStatus = useCallback(async (taskId: string, docId: string, currentDocsList?: DocumentSummary[]) => {
    if (!session) return;

    try {
      const res = await axios.get<TaskStatusResponse>(`/api/document-content/tasks/${taskId}/status`, {
        validateStatus: (status) => (status >= 200 && status < 300) || status === 404
      });

      if (res.status === 404) {
        // Fallback: Use the document's is_ready flag as the durable readiness signal
        const targetList = currentDocsList || [];
        const doc = targetList.find((d) => d.document_id === docId);
        const resolvedStatus = doc?.is_ready ? 'successful' : 'failed';

        setTaskStatuses((prev) => ({
          ...prev,
          [taskId]: {
            task_id: taskId,
            document_id: docId,
            status: resolvedStatus,
            error: doc?.is_ready ? null : 'Processing status expired and document is not ready.',
            updated_at: new Date().toISOString(),
          },
        }));
        return;
      }

      const data = res.data;
      setTaskStatuses((prev) => ({
        ...prev,
        [taskId]: data,
      }));
    } catch (err: unknown) {
      console.error(`Error fetching task status for ${taskId}:`, err);
      
      // Fallback in case of general status request network failure
      const targetList = currentDocsList || [];
      const doc = targetList.find((d) => d.document_id === docId);
      if (doc) {
        setTaskStatuses((prev) => ({
          ...prev,
          [taskId]: {
            task_id: taskId,
            document_id: docId,
            status: doc.is_ready ? 'successful' : 'failed',
            error: 'Failed to retrieve real-time status details.',
            updated_at: new Date().toISOString(),
          },
        }));
      }
    }
  }, [session]);

  // Fetch the current user's uploaded documents
  const fetchDocuments = useCallback(async (showLoading = true) => {
    if (!session) return;
    if (showLoading) setIsDocsLoading(true);
    setDocsError(null);

    try {
      const res = await axios.get('/api/document-content/documents');

      const data = res.data;
      const docs: DocumentSummary[] = data.documents || [];
      setDocuments(docs);

      // Eagerly fetch status for non-ready (pending) documents in parallel
      docs.forEach((doc) => {
        if (!doc.is_ready) {
          fetchTaskStatus(doc.task_id, doc.document_id, docs);
        }
      });
    } catch (err: unknown) {
      setDocsError(err instanceof Error ? err.message : 'Error loading documents');
    } finally {
      if (showLoading) setIsDocsLoading(false);
    }
  }, [session, fetchTaskStatus]);

  // Fetch starred games
  const fetchStarredGames = useCallback(async (cursor: string | null = null, append = false) => {
    if (!session) return;
    if (!cursor) setIsStarredLoading(true);
    setStarredError(null);

    try {
      const url = `/api/document-content/library/starred` + (cursor ? `?cursor=${cursor}` : '');
      const res = await axios.get(url);
      const data = res.data;
      const games: GameLibrarySummary[] = data.games || [];

      if (append) {
        setStarredGames((prev) => [...prev, ...games]);
      } else {
        setStarredGames(games);
      }
      setStarredCursor(data.next_cursor || null);
      setStarredHasMore(!!data.next_cursor);
    } catch (err: unknown) {
      setStarredError(err instanceof Error ? err.message : 'Error loading starred games');
    } finally {
      setIsStarredLoading(false);
    }
  }, [session]);

  // Fetch public games
  const fetchPublicGames = useCallback(async (cursor: string | null = null, append = false) => {
    if (!session) return;
    if (!cursor) setIsPublicLoading(true);
    setPublicError(null);

    try {
      const url = `/api/document-content/library/games` + (cursor ? `?cursor=${cursor}` : '');
      const res = await axios.get(url);
      const data = res.data;
      const games: GameLibrarySummary[] = data.games || [];

      if (append) {
        setPublicGames((prev) => [...prev, ...games]);
      } else {
        setPublicGames(games);
      }
      setPublicCursor(data.next_cursor || null);
      setPublicHasMore(!!data.next_cursor);
    } catch (err: unknown) {
      setPublicError(err instanceof Error ? err.message : 'Error loading library games');
    } finally {
      setIsPublicLoading(false);
    }
  }, [session]);

  // Delete a document and its generated game content
  const handleDeleteDocument = useCallback(async (docId: string) => {
    if (!session) return;
    setIsDeletingInProgress(true);

    try {
      await axios.delete(`/api/document-content/documents/${docId}`);

      setDeletingDocId(null);
      fetchDocuments(false);
      // Clean up starred games in background if it's there
      setStarredGames((prev) => prev.filter((g) => g.document_id !== docId));
      // Clean up public games list in background if it's there
      setPublicGames((prev) => prev.filter((g) => g.document_id !== docId));
    } catch (err: unknown) {
      console.error('Error deleting document:', err);
      alert(err instanceof Error ? err.message : 'Error occurred while deleting document');
    } finally {
      setIsDeletingInProgress(false);
    }
  }, [session, fetchDocuments]);

  // Toggle Visibility handler
  const handleToggleVisibility = useCallback(async (docId: string, currentPublic: boolean) => {
    if (!session) return;
    try {
      await axios.patch(`/api/document-content/documents/${docId}/visibility`, {
        is_public: !currentPublic
      });
      // Update owned documents state
      setDocuments((prev) =>
        prev.map((d) => (d.document_id === docId ? { ...d, is_public: !currentPublic } : d))
      );
      // Update starred games state if present
      setStarredGames((prev) =>
        prev.map((g) => (g.document_id === docId ? { ...g, is_public: !currentPublic } : g))
      );
      // Update public games list if present. If it was public and made private, remove it from the library list.
      if (currentPublic) {
        setPublicGames((prev) => prev.filter((g) => g.document_id !== docId));
      } else {
        setPublicGames((prev) =>
          prev.map((g) => (g.document_id === docId ? { ...g, is_public: true } : g))
        );
      }
    } catch (err: unknown) {
      console.error('Error updating document visibility:', err);
      alert(err instanceof Error ? err.message : 'Failed to update visibility status');
    }
  }, [session]);

  // Toggle Star / Unstar handler
  const handleToggleStar = useCallback(async (documentId: string, currentStarred: boolean) => {
    if (!session) return;
    try {
      if (currentStarred) {
        await axios.delete(`/api/document-content/library/games/${documentId}/star`);
        // Remove from starred games list
        setStarredGames((prev) => prev.filter((g) => g.document_id !== documentId));
        // Update public games list star status
        setPublicGames((prev) =>
          prev.map((g) => (g.document_id === documentId ? { ...g, starred_by_me: false } : g))
        );
      } else {
        await axios.post(`/api/document-content/library/games/${documentId}/star`);
        // Update public games list star status
        setPublicGames((prev) =>
          prev.map((g) => (g.document_id === documentId ? { ...g, starred_by_me: true } : g))
        );
        // Refresh starred games list in background
        fetchStarredGames(null, false);
      }
    } catch (err: unknown) {
      console.error('Error toggling star status:', err);
      alert(err instanceof Error ? err.message : 'Failed to update star status');
    }
  }, [session, fetchStarredGames]);

  // Authentication and Session monitoring
  useEffect(() => {
    let isMounted = true;
    const supabase = getSupabaseBrowserClient();

    supabase.auth.getSession().then(async ({ data, error }) => {
      if (!isMounted) return;

      if (error) {
        setAuthError(error.message);
      }

      if (data.session) {
        await syncAuthCookie(data.session);
        await upsertAuthenticatedUserProfile(data.session).catch((error) => {
          console.error('Failed to store user profile:', error);
        });
        setSession(data.session);
        setIsLoading(false);
      } else {
        await clearAuthCookie();
        // No session found, bounce to login
        router.push('/');
      }
    });

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange(async (_event, nextSession) => {
      if (!isMounted) return;

      await syncAuthCookie(nextSession);
      await upsertAuthenticatedUserProfile(nextSession).catch((error) => {
        console.error('Failed to store user profile:', error);
      });
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

  // Load documents when session is ready
  useEffect(() => {
    if (session) {
      // Defer execution using setTimeout to avoid calling setState synchronously inside the effect
      const timer = setTimeout(() => {
        fetchDocuments(true);
      }, 0);
      return () => clearTimeout(timer);
    }
  }, [session, fetchDocuments]);

  // Load starred games when tab switches to starred-games
  useEffect(() => {
    if (session && activeTab === 'starred-games') {
      fetchStarredGames(null, false);
    }
  }, [session, activeTab, fetchStarredGames]);

  // Load public games when active main tab is 'library'
  useEffect(() => {
    if (session && activeMainTab === 'library') {
      fetchPublicGames(null, false);
    }
  }, [session, activeMainTab, fetchPublicGames]);

  // Set up 5-second polling interval for any active (queued or processing) tasks
  useEffect(() => {
    if (documents.length === 0) return;

    const activeDocs = documents.filter((doc) => {
      if (doc.is_ready) return false;
      const statusObj = taskStatuses[doc.task_id];
      if (!statusObj) {
        return true;
      }
      return statusObj.status === 'queued' || statusObj.status === 'processing';
    });

    if (activeDocs.length === 0) return;

    const interval = setInterval(() => {
      activeDocs.forEach((doc) => {
        fetchTaskStatus(doc.task_id, doc.document_id, documents);
      });
      // Silently refresh document summaries to keep is_ready synchronized in background
      fetchDocuments(false);
    }, 5000);

    return () => clearInterval(interval);
  }, [documents, taskStatuses, fetchDocuments, fetchTaskStatus]);

  async function handleSignOut() {
    const supabase = getSupabaseBrowserClient();
    setIsSubmitting(true);
    setAuthError(null);

    const { error } = await supabase.auth.signOut();
    await clearAuthCookie();

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
    <div className="flex flex-col flex-1 items-center justify-center font-sans p-4 select-none">
      <Image src="/skybloomtd-logo.png" alt="SkybloomTD" width={400} height={400} className="mb-6 drop-shadow-lg" />
      
      {/* Folder-style Main Tabs */}
      <div className="flex gap-1.5 justify-start w-full max-w-5xl -mb-[14px] px-10 select-none relative z-0">
        <button
          type="button"
          onClick={() => setActiveMainTab('dashboard')}
          className={`${
            activeMainTab === 'dashboard' ? tabStyles.tabActive : tabStyles.tab
          } px-8 py-2 text-2xl active:scale-95 transition-all duration-100`}
        >
          Dashboard
        </button>
        <button
          type="button"
          onClick={() => setActiveMainTab('library')}
          className={`${
            activeMainTab === 'library' ? tabStyles.tabActive : tabStyles.tab
          } px-8 py-2 text-2xl active:scale-95 transition-all duration-100`}
        >
          Public Games
        </button>
      </div>

      <OrangeSquare className="w-full max-w-5xl flex flex-col items-center justify-center py-10 px-8 relative z-10">
        <h1 className="text-4xl text-white mb-6 drop-shadow-[0_2px_0_rgba(0,0,0,0.25)] tracking-wider">
          {activeMainTab === 'dashboard' ? 'Player Dashboard' : 'Public Games'}
        </h1>

        {activeMainTab === 'dashboard' ? (
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 w-full">
            {/* Left Column: Player Profile (lg:col-span-5) */}
            <div className="lg:col-span-5 flex flex-col items-center gap-6 w-full">
              <BlueSquare className="w-full flex flex-col items-center justify-center py-6 px-4">
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
            </div>

            {/* Right Column: Documents and Realtime Status (lg:col-span-7) */}
            <div className="lg:col-span-7 flex flex-col gap-6 w-full">
              <BlueSquare className="w-full flex flex-col p-6 min-h-[300px]">
                {/* Tab Navigation */}
                <div className="flex items-center gap-6 border-b-2 border-yellow-600/30 pb-3 mb-4 select-none">
                  <button
                    type="button"
                    onClick={() => setActiveTab('my-games')}
                    className={`text-2xl font-bold cursor-pointer pb-3 -mb-[14px] border-b-2 ${
                      activeTab === 'my-games'
                        ? 'text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] border-yellow-300'
                        : 'text-white/60 hover:text-white/80 border-transparent'
                    }`}
                  >
                    My Games
                  </button>
                  <button
                    type="button"
                    onClick={() => setActiveTab('starred-games')}
                    className={`text-2xl font-bold cursor-pointer pb-3 -mb-[14px] border-b-2 ${
                      activeTab === 'starred-games'
                        ? 'text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] border-yellow-300'
                        : 'text-white/60 hover:text-white/80 border-transparent'
                    }`}
                  >
                    Starred Games
                  </button>
                </div>

                {/* Scroll list container */}
                <div className="flex-1 overflow-y-auto max-h-[360px] pr-1 flex flex-col gap-3">
                  {activeTab === 'my-games' ? (
                    isDocsLoading && documents.length === 0 ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-12 gap-3 text-center text-white/70 font-bold select-none">
                        <Loader2 className="w-8 h-8 text-yellow-300 animate-spin" />
                        <div className="text-xl animate-pulse">Loading games database...</div>
                      </div>
                    ) : docsError ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-8 text-center text-red-300 select-none">
                        <AlertTriangle className="w-10 h-10 text-red-400 mb-2" />
                        <p className="font-semibold">{docsError}</p>
                        <button
                          onClick={() => fetchDocuments(true)}
                          className="mt-3 text-xs underline text-yellow-300 hover:text-yellow-400 font-bold cursor-pointer"
                        >
                          Try Again
                        </button>
                      </div>
                    ) : documents.length === 0 ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-12 text-center text-white/60 select-none">
                        <Gamepad2 className="w-12 h-12 text-yellow-300 mb-3 animate-pulse" />
                        <p className="font-bold text-lg text-yellow-200">
                          Upload your study notes to create a game
                        </p>
                      </div>
                    ) : (
                      documents.map((doc) => {
                        const statusObj = taskStatuses[doc.task_id];
                        const status = statusObj?.status || (doc.is_ready ? 'successful' : 'queued');
                        const errorMsg = statusObj?.error;

                        return (
                          <GameCardItem
                            key={doc.document_id}
                            doc={doc}
                            status={status}
                            errorMsg={errorMsg}
                            deletingDocId={deletingDocId}
                            setDeletingDocId={setDeletingDocId}
                            isDeletingInProgress={isDeletingInProgress}
                            onDelete={handleDeleteDocument}
                            onToggleVisibility={handleToggleVisibility}
                            onPlay={(docId) => router.push(`/dashboard/games/${docId}/chapters`)}
                          />
                        );
                      })
                    )
                  ) : (
                    /* Starred Games Tab content */
                    isStarredLoading && starredGames.length === 0 ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-12 gap-3 text-center text-white/70 font-bold select-none">
                        <Loader2 className="w-8 h-8 text-yellow-300 animate-spin" />
                        <div className="text-xl animate-pulse">Loading starred games...</div>
                      </div>
                    ) : starredError ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-8 text-center text-red-300 select-none">
                        <AlertTriangle className="w-10 h-10 text-red-400 mb-2" />
                        <p className="font-semibold">{starredError}</p>
                        <button
                          onClick={() => fetchStarredGames(null, false)}
                          className="mt-3 text-xs underline text-yellow-300 hover:text-yellow-400 font-bold cursor-pointer"
                        >
                          Try Again
                        </button>
                      </div>
                    ) : starredGames.length === 0 ? (
                      <div className="flex flex-col flex-1 items-center justify-center py-12 text-center text-white/60 select-none">
                        <Gamepad2 className="w-12 h-12 text-yellow-300 mb-3 animate-pulse" />
                        <p className="font-bold text-lg text-yellow-200">
                          No starred games yet. Browse public games in the library!
                        </p>
                      </div>
                    ) : (
                      <div className="flex flex-col gap-3">
                        {starredGames.map((game) => (
                          <LibraryGameCard
                            key={game.document_id}
                            game={game}
                            currentUserId={session.user.id}
                            onToggleStar={handleToggleStar}
                            onPlay={(docId) => router.push(`/dashboard/games/${docId}/chapters`)}
                            onToggleVisibility={handleToggleVisibility}
                            onDelete={handleDeleteDocument}
                            isDeletingInProgress={isDeletingInProgress}
                          />
                        ))}

                        {starredHasMore && (
                          <button
                            type="button"
                            onClick={() => fetchStarredGames(starredCursor, true)}
                            disabled={isStarredLoading}
                            className="w-full mt-2 py-2 bg-yellow-600/10 hover:bg-yellow-600/20 border border-yellow-600/30 rounded text-yellow-300 text-sm font-bold active:scale-98 transition-all cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed select-none"
                          >
                            {isStarredLoading ? 'Loading more...' : 'Load More Starred'}
                          </button>
                        )}
                      </div>
                    )
                  )}
                </div>

                {/* Action buttons under list */}
                <div className="flex gap-4 items-center justify-between mt-5 pt-3 border-t border-yellow-600/20">
                  <button
                    type="button"
                    onClick={() => {
                      if (activeTab === 'my-games') {
                        fetchDocuments(true);
                      } else {
                        fetchStarredGames(null, false);
                      }
                    }}
                    disabled={activeTab === 'my-games' ? isDocsLoading : isStarredLoading}
                    className="flex items-center gap-2 text-xs text-yellow-300 hover:text-yellow-400 font-bold bg-[#4a1900]/25 border border-yellow-600/20 rounded-lg px-3.5 py-2 cursor-pointer transition-all duration-150 active:scale-95 disabled:opacity-50 select-none"
                  >
                    <RefreshCw className={`w-3.5 h-3.5 ${((activeTab === 'my-games' ? isDocsLoading : isStarredLoading) ? 'animate-spin' : '')}`} /> 
                    {((activeTab === 'my-games' ? isDocsLoading : isStarredLoading) ? 'Refreshing...' : 'Refresh List')}
                  </button>

                  <ButtonGreen
                    className="h-10 px-5 text-lg text-white drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] hover:brightness-110 active:scale-95 transition-all duration-100 select-none"
                    onClick={() => setIsUploadOpen(true)}
                  >
                    <span className="text-white">Upload Document</span>
                  </ButtonGreen>
                </div>
              </BlueSquare>
            </div>
          </div>
        ) : (
          /* Game Library Tab content */
          <div className="w-full flex flex-col items-stretch">
            <BlueSquare className="w-full flex flex-col p-6 min-h-[400px]">
              <div className="flex items-center justify-between border-b-2 border-yellow-600/30 pb-3 mb-4 select-none">
                <h2 className="text-2xl text-yellow-300 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] font-bold">
                  Browse Shared Games
                </h2>
                
                <button
                  type="button"
                  onClick={() => fetchPublicGames(null, false)}
                  disabled={isPublicLoading}
                  className="flex items-center gap-1.5 text-xs text-yellow-300 hover:text-yellow-400 font-bold bg-[#4a1900]/20 border border-yellow-600/20 rounded px-2.5 py-1.5 cursor-pointer transition-all duration-150 active:scale-95 disabled:opacity-50 select-none"
                >
                  <RefreshCw className={`w-3.5 h-3.5 ${isPublicLoading ? 'animate-spin' : ''}`} /> Refresh Library
                </button>
              </div>

              {/* Scroll list container */}
              <div className="flex-1 overflow-y-auto max-h-[420px] pr-1 flex flex-col gap-3">
                {isPublicLoading && publicGames.length === 0 ? (
                  <div className="flex flex-col flex-1 items-center justify-center py-16 gap-3 text-center text-white/70 font-bold select-none">
                    <Loader2 className="w-10 h-10 text-yellow-300 animate-spin" />
                    <div className="text-xl animate-pulse">Loading public games database...</div>
                  </div>
                ) : publicError ? (
                  <div className="flex flex-col flex-1 items-center justify-center py-10 text-center text-red-300 select-none">
                    <AlertTriangle className="w-12 h-12 text-red-400 mb-2" />
                    <p className="font-semibold text-lg">{publicError}</p>
                    <button
                      onClick={() => fetchPublicGames(null, false)}
                      className="mt-3 text-xs underline text-yellow-300 hover:text-yellow-400 font-bold cursor-pointer"
                    >
                      Try Again
                    </button>
                  </div>
                ) : publicGames.length === 0 ? (
                  <div className="flex flex-col flex-1 items-center justify-center py-16 text-center text-white/60 select-none">
                    <Gamepad2 className="w-14 h-14 text-yellow-300 mb-3 animate-pulse" />
                    <p className="font-bold text-xl text-yellow-200">
                      No public games found
                    </p>
                    <p className="text-sm mt-1 text-white/40">
                      Be the first to publish a game by marking it Public!
                    </p>
                  </div>
                ) : (
                  <div className="flex flex-col gap-3">
                    {publicGames.map((game) => (
                      <LibraryGameCard
                        key={game.document_id}
                        game={game}
                        currentUserId={session.user.id}
                        onToggleStar={handleToggleStar}
                        onPlay={(docId) => router.push(`/dashboard/games/${docId}/chapters`)}
                        onToggleVisibility={handleToggleVisibility}
                        onDelete={handleDeleteDocument}
                        isDeletingInProgress={isDeletingInProgress}
                      />
                    ))}

                    {publicHasMore && (
                      <button
                        type="button"
                        onClick={() => fetchPublicGames(publicCursor, true)}
                        disabled={isPublicLoading}
                        className="w-full mt-2 py-2 bg-yellow-600/10 hover:bg-yellow-600/20 border border-yellow-600/30 rounded text-yellow-300 text-sm font-bold active:scale-98 transition-all cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed select-none"
                      >
                        {isPublicLoading ? 'Loading more...' : 'Load More Games'}
                      </button>
                    )}
                  </div>
                )}
              </div>
            </BlueSquare>
          </div>
        )}
      </OrangeSquare>

      <UploadDocumentModal 
        isOpen={isUploadOpen}
        onClose={() => setIsUploadOpen(false)}
        onUploadSuccess={() => {
          // Immediately fetch documents list on successful queue
          fetchDocuments(false);
        }}
      />
    </div>
  );
}
