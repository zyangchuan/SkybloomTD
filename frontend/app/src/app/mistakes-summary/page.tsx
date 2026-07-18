'use client';

import React, { Suspense, useEffect, useState, useRef } from 'react';
import { useSearchParams } from 'next/navigation';

interface QuizMistake {
  id: string;
  level_id: string;
  generation_id: string;
  quiz_id: string;
  quiz_index: number;
  quiz_type: 'mcq' | 'true_false';
  question_markdown: string;
  options_markdown: string[];
  answer_index: number;
  selected_index: number;
  correct_option_markdown: string;
  selected_option_markdown: string;
  created_at: string;
}

type RenderMathInElement = (
  element: HTMLElement,
  options: {
    delimiters: Array<{ left: string; right: string; display: boolean }>;
    throwOnError: boolean;
  }
) => void;

function MistakesSummaryContent() {
  const searchParams = useSearchParams();
  const isMobile = searchParams.get('mobile') === 'true';
  const sessionId = searchParams.get('session_id') || '';
  const victory = searchParams.get('victory') === 'true';

  const [mistakes, setMistakes] = useState<QuizMistake[]>([]);
  const [loading, setLoading] = useState(() => Boolean(sessionId));
  const [katexReady, setKatexReady] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Fetch Mistakes list from API
  useEffect(() => {
    if (!sessionId) {
      return;
    }

    async function fetchMistakes() {
      try {
        const response = await fetch(`/api/game-service/quiz-mistakes?session_id=${sessionId}`);
        if (response.ok) {
          const data = await response.json();
          setMistakes(data.mistakes || []);
        } else {
          console.error('Failed to fetch mistakes from API:', response.status);
        }
      } catch (err) {
        console.error('Failed to retrieve quiz mistakes summary:', err);
      } finally {
        setLoading(false);
      }
    }

    fetchMistakes();
  }, [sessionId]);

  // Preload KaTeX libraries dynamically inside the iframe context
  useEffect(() => {
    // Preload stylesheet
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/katex.min.css';
    document.head.appendChild(link);

    // Preload KaTeX core parser
    const scriptJs = document.createElement('script');
    scriptJs.src = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/katex.min.js';
    scriptJs.async = false;
    scriptJs.onload = () => {
      // Preload auto-render extension
      const scriptAuto = document.createElement('script');
      scriptAuto.src = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/contrib/auto-render.min.js';
      scriptAuto.async = false;
      scriptAuto.onload = () => {
        setKatexReady(true);
      };
      document.head.appendChild(scriptAuto);
    };
    document.head.appendChild(scriptJs);

    return () => {
      link.remove();
      scriptJs.remove();
    };
  }, []);

  // Run KaTeX auto-typesetter when content or script is ready
  useEffect(() => {
    const renderMathInElement = (window as Window & { renderMathInElement?: RenderMathInElement }).renderMathInElement;
    if (katexReady && containerRef.current && renderMathInElement) {
      try {
        renderMathInElement(containerRef.current, {
          delimiters: [
            { left: '$$', right: '$$', display: true },
            { left: '$', right: '$', display: false },
            { left: '\\(', right: '\\)', display: false },
            { left: '\\[', right: '\\]', display: true }
          ],
          throwOnError: false
        });
      } catch (e) {
        console.error('KaTeX typesetting crashed in summary', e);
      }
    }
  }, [katexReady, mistakes, loading]);

  const handleReplay = () => {
    window.parent.postMessage({ type: 'game-replay', sessionId }, '*');
  };

  const handleExit = () => {
    window.parent.postMessage({ type: 'game-exit', sessionId }, '*');
  };

  return (
    <div 
      ref={containerRef}
      className={isMobile
        ? "fixed inset-0 w-full h-full flex items-center justify-center p-1 bg-transparent select-none overflow-hidden"
        : "fixed inset-0 w-full h-full flex items-center justify-center p-4 bg-transparent select-none overflow-hidden"
      }
    >
      <style>{`
        /* Force transparent document backdrops inside Next.js layout */
        html, body {
          background: transparent !important;
          background-image: none !important;
        }

        /* Hide moving root cloud banners */
        .sky-clouds {
          display: none !important;
        }

        /* Thinner mobile-only 9-slice outlines and borders */
        ${isMobile ? `
          .modal-9slice {
            border-width: 12px !important;
            border-image-width: 12px !important;
          }
          .mistake-card-9slice {
            border-width: 8px !important;
            border-image-width: 8px !important;
          }
          .btn-play-again-9slice {
            border-width: 8px !important;
            border-image-width: 8px !important;
          }
          .btn-exit-9slice {
            border-width: 8px !important;
            border-image-width: 8px !important;
          }
        ` : ''}

        /* Custom 9-slice container overlays matching Phaser boxes */
        .modal-9slice {
          border: 24px solid transparent;
          border-image-source: url('/game/assets/gui/boxes_banners/Box_Orange_Square.svg');
          border-image-slice: 64 fill;
          border-image-width: 24px;
          border-image-repeat: stretch;
        }

        .mistake-card-9slice {
          border: 12px solid transparent;
          border-image-source: url('/game/assets/gui/boxes_banners/Box_Square.svg');
          border-image-slice: 64 fill;
          border-image-width: 12px;
          border-image-repeat: stretch;
          background: transparent !important;
        }

        /* Block stacking KaTeX formula elements cleanly */
        .katex {
          display: block !important;
          text-align: center !important;
          margin: 6px 0 !important;
        }

        /* Custom thin visual scroll cue scrollbars */
        .scroll-custom::-webkit-scrollbar {
          width: 6px;
        }
        .scroll-custom::-webkit-scrollbar-track {
          background: rgba(0, 0, 0, 0.08);
          border-radius: 4px;
        }
        .scroll-custom::-webkit-scrollbar-thumb {
          background: rgba(120, 53, 4, 0.35); /* Brown orange scroll cue */
          border-radius: 4px;
          transition: background 0.2s ease;
        }
        .scroll-custom::-webkit-scrollbar-thumb:hover {
          background: rgba(120, 53, 4, 0.6);
        }
        .scroll-custom {
          scrollbar-width: thin;
          scrollbar-color: rgba(120, 53, 4, 0.35) rgba(0, 0, 0, 0.08);
        }

        /* Dynamic premium 9-slice buttons matching Phaser rounded buttons */
        .btn-play-again-9slice {
          border: 12px solid transparent;
          border-image-source: url('/game/assets/gui/buttons_text/ButtonText_Small_Blue_Round.svg');
          border-image-slice: 64 fill;
          border-image-width: 12px;
          border-image-repeat: stretch;
          background: transparent !important;
          transition: transform 0.15s ease;
        }
        .btn-play-again-9slice:hover {
          transform: scale(1.05);
          cursor: pointer;
        }
        .btn-play-again-9slice:active {
          transform: scale(0.95);
        }

        .btn-exit-9slice {
          border: 12px solid transparent;
          border-image-source: url('/game/assets/gui/buttons_text/ButtonText_Small_Blank_Round.svg');
          border-image-slice: 64 fill;
          border-image-width: 12px;
          border-image-repeat: stretch;
          background: transparent !important;
          transition: transform 0.15s ease;
        }
        .btn-exit-9slice:hover {
          transform: scale(1.05);
          cursor: pointer;
        }
        .btn-exit-9slice:active {
          transform: scale(0.95);
        }

        /* Native responsive height flexbox layout */
        .mistakes-modal-container {
          width: 95%;
          max-width: 580px; /* Cozy smaller width on mobile landscape */
          height: 96vh;
          height: 96dvh; /* Let the height grow fully to occupy screen vertically */
          display: flex;
          flex-direction: column;
          justify-content: space-between;
          align-items: center;
          box-sizing: border-box;
          padding: 4px 12px 6px 12px !important; /* Even smaller paddings */
          overflow: hidden;
          position: relative;
        }

        .mistakes-list-container {
          width: 100%;
          flex: 1; /* Dynamic flexible height! */
          margin: 6px 0; /* Smaller margin */
          padding: 0 8px;
          box-sizing: border-box;
          overflow-y: auto;
        }

        @media (min-height: 750px) {
          .mistakes-modal-container {
            height: 680px; /* Keep clean original height on standard viewports */
            max-width: 780px; /* Restore desktop wider max-width if high resolution height is active */
          }
        }
      `}</style>

      {/* Main Container Window Box with Orange Square nineslice outline styling */}
      <div className={isMobile
        ? "modal-9slice mistakes-modal-container z-10 text-amber-950"
        : "modal-9slice w-[780px] max-w-[95%] h-[680px] flex flex-col justify-between items-center p-6 relative z-10 box-border text-amber-950"
      }>
        
        {/* Upper Header Title & Level Subtext */}
        <div className="text-center w-full mt-2">
          <h1 className={`font-concert ${isMobile ? 'text-3xl mt-1' : 'text-6xl mt-2'} tracking-wider select-none ${victory ? 'text-yellow-300 drop-shadow-sm' : 'text-red-950 drop-shadow-[0_1px_2px_rgba(255,255,255,0.2)]'}`}>
            {victory ? 'VICTORY' : 'DEFEATED'}
          </h1>
          <p className={`font-concert text-amber-900 font-bold ${isMobile ? 'text-xs mt-1' : 'text-lg mt-2'} tracking-wide`}>
            {victory 
              ? 'You have successfully protected the skies!' 
              : 'The smogs have overwhelmed your defenses.'
            }
          </p>
        </div>

        {/* Dynamic mistakes review body list */}
        <div className={isMobile
          ? "mistakes-list-container scroll-custom flex flex-col gap-2"
          : "w-full flex-1 my-4 px-2 overflow-y-auto scroll-custom flex flex-col gap-4 max-h-[360px] box-border"
        }>
          {loading ? (
            <div className="flex flex-col items-center justify-center h-48 gap-3">
              <div className="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 border-amber-900" />
              <span className="font-concert text-amber-950 text-lg font-bold">Analyzing mistakes summary...</span>
            </div>
          ) : mistakes.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-10 text-center gap-4 bg-emerald-900/10 border border-emerald-950/20 rounded-2xl p-6">
              <div className="animate-bounce">
                <img src="/gui/icons/Icon_Large_Star.svg" className="w-16 h-16 object-contain" alt="" />
              </div>
              <h2 className="font-concert text-emerald-900 text-2xl font-bold tracking-wide">
                FLAWLESS RUN!
              </h2>
              <p className="font-concert text-emerald-950 text-base max-w-[400px]">
                You made absolute zero mistakes! All quizzes were solved with perfect accuracy. Stellar job!
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-4">
              <h2 className={`font-concert text-amber-950 ${isMobile ? 'text-sm mb-1 pb-1' : 'text-lg mb-2 pb-2'} font-bold border-b border-amber-950/30 select-none tracking-wide text-left`}>
                Mistakes Review ({mistakes.length})
              </h2>
              {mistakes.map((mistake, mIdx) => {
                // Ensure options are parsed safely
                const options = mistake.options_markdown || (mistake.quiz_type === 'true_false' ? ['True', 'False'] : []);

                return (
                  <div 
                    key={mistake.id} 
                    className={isMobile 
                      ? "mistake-card-9slice p-2.5 flex flex-col gap-2 transition duration-150 text-left"
                      : "mistake-card-9slice p-4 flex flex-col gap-3 transition duration-150 text-left"
                    }
                  >
                    {/* Math Question Text */}
                    <div className={`font-concert text-slate-200 ${isMobile ? 'text-xs' : 'text-base'} font-medium break-words leading-relaxed`}>
                      <span className="text-slate-400 font-bold mr-1">Q{mIdx + 1}:</span> {mistake.question_markdown}
                    </div>

                    {/* MCQs options cards block */}
                    <div className="grid grid-cols-1 gap-2 mt-1">
                      {options.map((opt, oIdx) => {
                        const isSelected = oIdx === mistake.selected_index;
                        const isCorrect = oIdx === mistake.answer_index;

                        let styleClass = "border-slate-800/80 bg-slate-950/40 text-slate-300";
                        let prefix = "";

                        if (isCorrect) {
                          styleClass = "border-emerald-500/40 bg-emerald-500/10 text-emerald-300 font-bold";
                          prefix = "✓ ";
                        } else if (isSelected) {
                          styleClass = "border-rose-500/40 bg-rose-500/10 text-rose-300 font-bold";
                          prefix = "✗ ";
                        }

                        return (
                          <div 
                            key={oIdx} 
                            className={`border ${isMobile ? 'px-2 py-1.5 text-xs' : 'px-3.5 py-2 rounded-lg text-sm'} rounded-lg flex items-center justify-between transition duration-150 ${styleClass}`}
                          >
                            <span className="font-concert">{prefix}{opt}</span>
                            {isCorrect && (
                              <span className={`${isMobile ? 'text-[9px] px-1.5 py-0.2' : 'text-xs px-2.5 py-0.5'} text-emerald-400 font-bold uppercase tracking-wider bg-emerald-500/20 rounded`}>
                                Correct Answer
                              </span>
                            )}
                            {isSelected && !isCorrect && (
                              <span className={`${isMobile ? 'text-[9px] px-1.5 py-0.2' : 'text-xs px-2.5 py-0.5'} text-rose-400 font-bold uppercase tracking-wider bg-rose-500/20 rounded`}>
                                Your Selection
                              </span>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Bottom Interactive Play Actions */}
        <div className={isMobile
          ? "flex flex-row justify-center items-center gap-4 w-full mt-1 mb-1"
          : "flex flex-row justify-center items-center gap-6 w-full mt-2 mb-2"
        }>
          {/* Replay glowing button using ButtonText_Small_Blue_Round.svg 9-slice */}
          <button
            onClick={handleReplay}
            className={isMobile 
              ? "btn-play-again-9slice font-concert text-sm text-white w-[140px] h-[42px] flex items-center justify-center border-0 outline-none box-border"
              : "btn-play-again-9slice font-concert text-xl font-bold text-white w-[220px] h-[64px] flex items-center justify-center border-0 outline-none box-border"
            }
          >
            PLAY AGAIN
          </button>

          {/* Exit button using ButtonText_Small_Blank_Round.svg 9-slice */}
          <button
            onClick={handleExit}
            className={isMobile
              ? "btn-exit-9slice font-concert text-sm text-black w-[140px] h-[42px] flex items-center justify-center border-0 outline-none box-border"
              : "btn-exit-9slice font-concert text-xl font-bold text-black w-[220px] h-[64px] flex items-center justify-center border-0 outline-none box-border"
            }
          >
            EXIT GAME
          </button>
        </div>

      </div>
    </div>
  );
}

export default function MistakesSummaryPage() {
  return (
    <Suspense fallback={
      <div className="fixed inset-0 w-full h-full flex items-center justify-center bg-transparent select-none font-concert text-white text-2xl">
        Loading Summary Panel...
      </div>
    }>
      <MistakesSummaryContent />
    </Suspense>
  );
}
