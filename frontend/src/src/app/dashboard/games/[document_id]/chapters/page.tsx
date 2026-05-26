'use client';

import { useEffect, useState, use, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { 
  Gamepad2, 
  Loader2, 
  ArrowLeft,
  AlertTriangle,
  Play
} from 'lucide-react';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import OrangeSquare from '@/components/OrangeSquare';
import ChapterSprite from '@/components/ChapterSprite';
import ButtonOrange from '@/components/ButtonOrange';
import ButtonWhite from '@/components/ButtonWhite';
import ButtonSmallGreenRound from '@/components/ButtonSmallGreenRound';
import GameCardBackground from '@/components/GameCardBackground';

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
  }>;
}

export default function ChapterSelectionPage({ params }: PageProps) {
  const resolvedParams = use(params);
  const documentId = resolvedParams.document_id;
  const router = useRouter();

  const [chapters, setChapters] = useState<Chapter[]>([]);
  const [docInfo, setDocInfo] = useState<DocumentInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchChapterData = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const supabase = getSupabaseBrowserClient();
      const { data: { session } } = await supabase.auth.getSession();
      const token = session?.access_token;

      if (!token) {
        router.push('/');
        return;
      }

      // Fetch chapters list
      const chaptersRes = await fetch(`/api/document-content/documents/${documentId}/chapters`, {
        headers: {
          Authorization: `Bearer ${token}`
        }
      });

      if (!chaptersRes.ok) {
        if (chaptersRes.status === 404) {
          throw new Error("Game or study notes not found.");
        }
        throw new Error(`Failed to load chapters (Code ${chaptersRes.status})`);
      }

      const chaptersData = await chaptersRes.json();
      
      // Sort chapters by chapter_index or index sequence
      const sortedChapters = (chaptersData.chapters || []).sort((a: Chapter, b: Chapter) => {
        const idxA = a.chapter_index ?? 0;
        const idxB = b.chapter_index ?? 0;
        return idxA - idxB;
      });
      setChapters(sortedChapters);

      // Fetch documents list to find the matching game name
      const docsRes = await fetch('/api/document-content/documents', {
        headers: {
          Authorization: `Bearer ${token}`
        }
      });
      if (docsRes.ok) {
        const docsData = await docsRes.json();
        const found = (docsData.documents || []).find((d: DocumentInfo) => d.document_id === documentId);
        if (found) {
          setDocInfo(found);
        }
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "An unexpected error occurred loading chapters.");
    } finally {
      setIsLoading(false);
    }
  }, [documentId, router]);

  useEffect(() => {
    let active = true;
    if (active) {
      setTimeout(() => {
        fetchChapterData();
      }, 0);
    }
    return () => {
      active = false;
    };
  }, [fetchChapterData]);

  return (
    <main className="flex-1 w-full max-w-6xl mx-auto px-4 py-8 flex flex-col justify-center items-center">
      <OrangeSquare className="w-full flex flex-col p-6 sm:p-8 bg-[#fdfaf2] border-4 border-yellow-800 rounded-3xl shadow-2xl relative min-h-[500px]">
        {/* Navigation Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b-2 border-yellow-800/20 pb-4 mb-6">
          <ButtonWhite
            onClick={() => router.push('/dashboard')}
            className="h-10 px-4 text-xs font-extrabold text-[#4a1900] hover:brightness-110 active:scale-95 transition-all select-none self-start flex items-center gap-1.5 drop-shadow-[0_1px_0_rgba(0,0,0,0.15)]"
          >
            <ArrowLeft className="w-4 h-4 stroke-[3px] text-[#4a1900]" />
            <span className="text-[#4a1900]">Back to Dashboard</span>
          </ButtonWhite>
          
          <h2 className="text-xl sm:text-2xl text-[#4a1900] font-extrabold tracking-tight text-left drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
            {docInfo ? `Game: ${docInfo.game_name}` : "Level Selection"}
          </h2>
        </div>

        {/* Dynamic Content Frame */}
        {isLoading ? (
          <div className="flex flex-col flex-1 items-center justify-center py-20 gap-4 text-center text-[#4a1900] select-none font-sans">
            <Loader2 className="w-12 h-12 text-yellow-600 animate-spin" />
            <div className="text-xl font-bold animate-pulse">Unrolling scroll and loading chapters...</div>
          </div>
        ) : error ? (
          <div className="flex flex-col flex-1 items-center justify-center py-16 text-center text-red-800 select-none max-w-md mx-auto gap-3">
            <AlertTriangle className="w-12 h-12 text-red-600 animate-bounce" />
            <p className="font-bold text-lg">{error}</p>
            <ButtonOrange
              onClick={() => fetchChapterData()}
              className="px-4 py-2 font-bold text-md mt-2"
            >
              Retry
            </ButtonOrange>
          </div>
        ) : chapters.length === 0 ? (
          <div className="flex flex-col flex-1 items-center justify-center py-16 text-center text-[#4a1900]/60 select-none max-w-md mx-auto gap-4">
            <Gamepad2 className="w-16 h-16 text-yellow-600 animate-pulse" />
            <p className="font-bold text-xl text-[#4a1900]">No chapters found for this game</p>
            <p className="text-sm font-semibold">
              Chapters will appear here once the study notes are fully indexed.
            </p>
            <ButtonOrange
              onClick={() => router.push('/dashboard')}
              className="px-5 py-2 font-bold text-md mt-2"
            >
              Return to Dashboard
            </ButtonOrange>
          </div>
        ) : (
          <div className="flex flex-col flex-1 w-full gap-5">
            <div className="flex items-center gap-2 mb-2 select-none">
              <span className="w-2.5 h-6 bg-yellow-600 rounded-full" />
              <h3 className="text-2xl text-[#4a1900] font-extrabold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
                Select Chapter
              </h3>
            </div>

            {/* Retro Level Cards Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-y-30 gap-x-30 pt-10">
              {chapters.map((chapter, idx) => {
                const chapterNum = chapter.chapter_index !== null ? chapter.chapter_index : idx + 1;
                return (
                  <div
                    key={chapter.chapter_id}
                    onClick={() => {
                      router.push(`/dashboard/games/${documentId}/chapters/${chapter.chapter_id}`);
                    }}
                    className="flex flex-col items-center gap-0 transition-all duration-200 hover:scale-[1.02] active:scale-[0.98] cursor-pointer group relative pt-4"
                  >
                    {/* CSS Percentage-Spliced Sprite Component (Larger, outside/above the card!) */}
                    <div className="-ml-10 -mt-20 z-10 drop-shadow-[0_8px_16px_rgba(0,0,0,0.3)] group-hover:scale-110 group-hover:-translate-y-1 transition-all duration-300 pointer-events-none">
                      <ChapterSprite 
                        index={idx} 
                        size={360} 
                      />
                    </div>

                    {/* 9-sliced Card containing Title and Select Button */}
                    <GameCardBackground className="w-full flex flex-col items-center justify-between text-center gap-2.5 -mt-20 pt-12 min-h-[160px] relative z-0">
                      <div className="flex flex-col items-center gap-1 w-full px-1">
                        <h4 className="text-white text-lg font-extrabold line-clamp-2 min-h-[3rem] flex items-center justify-center group-hover:text-yellow-200 transition-colors px-2 leading-tight">
                          {chapter.title || "Untitled Chapter"}
                        </h4>
                      </div>

                      <ButtonSmallGreenRound
                        onClick={(e) => {
                          e.stopPropagation();
                          router.push(`/dashboard/games/${documentId}/chapters/${chapter.chapter_id}`);
                        }}
                        className="mt-2 w-full py-1.5 font-extrabold text-white active:scale-95 transition-all flex items-center justify-center gap-1.5 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
                      >
                        <Play className="w-3.5 h-3.5 fill-current text-white stroke-2" />
                        <p className="text-white">Select</p>
                      </ButtonSmallGreenRound>
                    </GameCardBackground>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </OrangeSquare>
    </main>
  );
}
