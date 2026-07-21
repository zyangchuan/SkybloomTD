'use client';

import { use, useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import Image from 'next/image';
import { Gamepad2, ArrowLeft, RefreshCw, Star } from 'lucide-react';
import axios from 'axios';
import OrangeSquare from '@/components/OrangeSquare';
import ButtonWhite from '@/components/ButtonWhite';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import { syncAuthCookie } from '@/lib/auth-cookie';
import ButtonOrangeRound from '@/components/ButtonOrangRound';
import ButtonBlueRound from '@/components/ButtonBlueRound';

interface SubChapter {
  sub_chapter_id: string;
  document_id: string;
  chapter_id: string;
  sub_chapter_index: number | null;
  title: string | null;
  start_line: number | null;
  end_line: number | null;
  created_at: string;
}

interface Chapter {
  chapter_id: string;
  document_id: string;
  chapter_index: number | null;
  title: string | null;
  start_line: number | null;
  end_line: number | null;
  created_at: string;
}

interface DocumentInfo {
  document_id: string;
  filename: string;
  game_name: string;
  is_ready: boolean;
}

interface PageProps {
  params: Promise<{
    document_id: string;
    chapter_id: string;
  }>;
}

export default function LevelsPage({ params }: PageProps) {
  const resolvedParams = use(params);
  const documentId = resolvedParams.document_id;
  const chapterId = resolvedParams.chapter_id;
  const router = useRouter();

  const [levels, setLevels] = useState<SubChapter[]>([]);
  const [activeChapter, setActiveChapter] = useState<Chapter | null>(null);
  const [docInfo, setDocInfo] = useState<DocumentInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedLevel, setSelectedLevel] = useState<SubChapter | null>(null);

  const fetchLevelsData = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const supabase = getSupabaseBrowserClient();
      const { data: { session } } = await supabase.auth.getSession();

      if (!session?.access_token) {
        router.push('/');
        return;
      }
      await syncAuthCookie(session);

      // Fetch sub-chapters (levels) of the chapter
      let levelsRes;
      try {
        levelsRes = await axios.get(`/api/document-content/chapters/${chapterId}/sub-chapters`);
      } catch (err: unknown) {
        const status = axios.isAxiosError(err) ? err.response?.status : undefined;
        if (status === 404) {
          throw new Error("Chapter levels not found.");
        }
        throw new Error(`Failed to load levels (Code ${status || 'network error'})`);
      }

      const levelsData = levelsRes.data;

      // Sort sub-chapters by sub_chapter_index or creation sequence
      const sortedLevels = (levelsData.sub_chapters || []).sort((a: SubChapter, b: SubChapter) => {
        const idxA = a.sub_chapter_index ?? 0;
        const idxB = b.sub_chapter_index ?? 0;
        return idxA - idxB;
      });
      setLevels(sortedLevels);

      // Fetch all chapters of the document to resolve the active chapter title
      try {
        const chaptersRes = await axios.get(`/api/document-content/documents/${documentId}/chapters`);
        const chaptersData = chaptersRes.data;
        const foundChapter = (chaptersData.chapters || []).find((c: Chapter) => c.chapter_id === chapterId);
        if (foundChapter) {
          setActiveChapter(foundChapter);
        }
      } catch (e) {
        console.error("Failed to load active chapter:", e);
      }

      // Fetch documents list to find the matching game name
      try {
        const docsRes = await axios.get('/api/document-content/documents');
        const docsData = docsRes.data;
        const foundDoc = (docsData.documents || []).find((d: DocumentInfo) => d.document_id === documentId);
        if (foundDoc) {
          setDocInfo(foundDoc);
        }
      } catch (e) {
        console.error("Failed to load document info:", e);
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "An unexpected error occurred loading levels.");
    } finally {
      setIsLoading(false);
    }
  }, [documentId, chapterId, router]);

  const startGame = (mode: "freeplay" | "normal") => {
    if (!selectedLevel) return;

    const params = new URLSearchParams({
      document_id: documentId,
      chapter_id: chapterId,
      sub_chapter_id: selectedLevel.sub_chapter_id,
      mode,
    })

    window.location.href = `/game/?${params.toString()}`;
  }

  useEffect(() => {
    let active = true;
    if (active) {
      setTimeout(() => {
        fetchLevelsData();
      }, 0);
    }
    return () => {
      active = false;
    };
  }, [fetchLevelsData]);

  return (
    <main className="flex-1 w-full max-w-4xl mx-auto px-4 py-8 flex flex-col justify-center items-center">
      <OrangeSquare className="w-full flex flex-col p-6 sm:p-8 bg-[#fdfaf2] border-4 border-yellow-800 rounded-3xl shadow-2xl relative min-h-[500px]">

        {/* Navigation Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b-2 border-yellow-800/20 pb-4 mb-6">
          <div className="flex items-center gap-4">
            <ButtonWhite
              onClick={() => router.push(`/dashboard/games/${documentId}/chapters`)}
              className="py-1 px-4 font-extrabold text-[12px] uppercase text-yellow-900 border-2 active:scale-95 transition-all flex items-center gap-1.5 shadow-md shrink-0"
            >
              <ArrowLeft className="w-3.5 h-3.5 stroke-[3px]" />
              <span>Back</span>
            </ButtonWhite>
            <div className="flex flex-col">
              <span className="text-[10px] sm:text-[11px] font-extrabold text-yellow-600 uppercase tracking-widest leading-none mb-1">
                {docInfo?.game_name || "Game Catalog"}
              </span>
              <h2 className="text-xl sm:text-2xl font-extrabold text-yellow-900 leading-tight">
                {activeChapter?.title || "Chapter Levels"}
              </h2>
            </div>
          </div>
        </div>

        {/* State Banners */}
        {isLoading ? (
          <div className="flex flex-col flex-1 items-center justify-center py-20 text-center select-none">
            <div className="relative w-16 h-16 flex items-center justify-center mb-4">
              <RefreshCw className="w-10 h-10 text-yellow-700 animate-spin" />
            </div>
            <p className="font-extrabold text-lg text-yellow-800">
              Loading Levels Path...
            </p>
          </div>
        ) : error ? (
          <div className="flex flex-col flex-1 items-center justify-center py-16 text-center">
            <div className="bg-red-50 border-2 border-red-200 rounded-2xl p-6 max-w-md shadow-lg flex flex-col items-center gap-3">
              <Gamepad2 className="w-12 h-12 text-red-500 animate-bounce" />
              <h3 className="font-extrabold text-lg text-red-800">Level Retrieval Failed</h3>
              <p className="text-sm text-red-600 leading-relaxed">{error}</p>
              <button
                onClick={fetchLevelsData}
                className="mt-2 bg-red-600 hover:bg-red-700 active:scale-95 transition-all text-white font-extrabold text-xs px-4 py-2 rounded-xl shadow-md cursor-pointer"
              >
                Retry Request
              </button>
            </div>
          </div>
        ) : levels.length === 0 ? (
          <div className="flex flex-col flex-1 items-center justify-center py-20 text-center select-none">
            <Gamepad2 className="w-16 h-16 text-yellow-700/40 mb-4 animate-pulse" />
            <p className="font-extrabold text-lg text-yellow-800">
              No Levels Available
            </p>
            <p className="text-sm text-yellow-600/80 max-w-sm mt-1 leading-relaxed">
              This chapter does not contain any study stages yet. Please re-upload or select another chapter.
            </p>
          </div>
        ) : (
          <div className="flex-1 flex flex-col items-center pt-6">

            {/* Legend / Title */}
            <div className="flex items-center gap-1.5 bg-yellow-800/10 border border-yellow-800/20 px-3.5 py-1.5 rounded-full mb-10 select-none">
              <Star className="w-4 h-4 text-yellow-600 fill-current" />
              <span className="text-[11px] font-extrabold text-yellow-800 uppercase tracking-wider">
                Select a stage to play
              </span>
            </div>

            {/* Staggered Path Map */}
            <div className="relative w-full py-12 flex flex-col items-center gap-20 min-h-[500px]">

              {/* Central Winding Dashed Path Connector Line */}
              <div className="absolute left-1/2 -translate-x-1/2 top-10 bottom-10 w-0 border-l-4 border-dashed border-yellow-800/30 z-0 pointer-events-none" />

              {levels.map((level, idx) => {
                const isLeft = idx % 2 === 0;
                const levelNum = level.sub_chapter_index !== null ? level.sub_chapter_index : idx + 1;

                return (
                  <div
                    key={level.sub_chapter_id}
                    onClick={() => {
                      setSelectedLevel(level);
                    }}
                    className={`relative z-10 flex flex-col items-center group transition-all duration-200 hover:scale-105 active:scale-95 cursor-pointer w-full max-w-[240px] select-none ${isLeft
                      ? '-translate-x-16 sm:-translate-x-28'
                      : 'translate-x-16 sm:translate-x-28'
                      }`}
                  >

                    {/* Horizontal Dashed Connector Arm to the central path */}
                    <div
                      className={`absolute top-10 h-0 border-t-4 border-dashed border-yellow-800/30 z-[-1] pointer-events-none ${isLeft
                        ? 'left-1/2 w-[64px] sm:w-[112px]'
                        : 'right-1/2 w-[64px] sm:w-[112px]'
                        }`}
                    />

                    {/* Circular Blue Icon Button with Level Index inside */}
                    <button
                      type="button"
                      className="w-20 h-20 flex items-center justify-center text-white hover:brightness-110 active:scale-95 transition-all cursor-pointer select-none bg-transparent shrink-0 drop-shadow-[0_6px_8px_rgba(0,0,0,0.25)] group-hover:drop-shadow-[0_8px_12px_rgba(0,0,0,0.35)]"
                      style={{
                        backgroundImage: "url('/gui/buttons_icons/IconButton_Large_Blue_Circle.svg')",
                        backgroundSize: '100% 100%',
                        backgroundPosition: 'center',
                        backgroundRepeat: 'no-repeat',
                      }}
                    >
                      <span className="text-2xl font-extrabold text-white drop-shadow-[0_2.5px_0_rgba(0,0,0,0.4)] -mt-1.5">
                        {levelNum}
                      </span>
                    </button>

                    {/* Level Title under the Circle */}
                    <span className="mt-3 text-xs sm:text-sm text-[#4a1900] font-extrabold text-center drop-shadow-[0_1px_0_rgba(255,255,255,0.4)] group-hover:text-yellow-900 transition-colors max-w-[160px] leading-tight break-words px-1">
                      {level.title || `Level ${levelNum}`}
                    </span>
                  </div>
                );
              })}

              {selectedLevel && (
                <div className="fixed inset-0 flex z-50 justify-center items-center bg-black/50 flex flex-col">
                  <OrangeSquare className="relative h-[300px] w-[520px] p-2">
                    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b-2 border-yellow-800/20 pb-2 mb-8">
                      <div className="flex items-center gap-4">
                        <ButtonWhite
                          onClick={() => setSelectedLevel(null)}
                          className="py-1 px-4 font-extrabold text-[12px] uppercase text-yellow-900 border-2 active:scale-95 transition-all flex items-center gap-1.5 shadow-md shrink-0"
                        >
                          <ArrowLeft className="w-3.5 h-3.5 stroke-[3px]" />
                          <span>Back</span>
                        </ButtonWhite>
                        <div className="flex flex-col">
                          <span className="text-[10px] sm:text-[11px] font-extrabold text-yellow-600 uppercase tracking-widest leading-none mb-1">
                            {docInfo?.game_name || "Game Catalog"}
                          </span>
                          <h2 className="text-xl sm:text-2xl font-extrabold text-yellow-900 leading-tight">
                            {activeChapter?.title || "Chapter Levels"}
                          </h2>
                        </div>
                      </div>
                    </div>

                    <div className="flex flex-col items-center gap-2">
                      <ButtonOrangeRound
                        onClick={() => startGame('normal')}
                        className="relative w-[200px] aspect-[1158/357] active:scale-95 transition-all flex justify-center items-center"
                      >

                        <span className="text-[22px] text-zinc-100"> Normal mode</span>

                      </ButtonOrangeRound>

                      <ButtonBlueRound
                        onClick={() => startGame('freeplay')}
                        className="relative w-[200px] aspect-[1158/357] active:scale-95 transition-all flex justify-center items-center"
                      >
                          <span className="text-[22px] text-zinc-100"> Free mode</span>

                      </ButtonBlueRound>

                    </div>
                  </OrangeSquare>

                </div>
              )}

            </div>

          </div>
        )}

      </OrangeSquare>
    </main>
  );
}
