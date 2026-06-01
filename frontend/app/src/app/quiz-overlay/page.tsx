'use client';

import React, { Suspense, useEffect, useState, useRef } from 'react';
import { useSearchParams } from 'next/navigation';

type QuizState = {
  quizId: string;
  question: string;
  options: string[];
  type: string;
};

type QuizPromptData = {
  quiz_id: string;
  question_markdown: string;
  options_markdown?: string[];
  quiz_type?: string;
};

type QuizResult = {
  quiz_id: string;
  correct: boolean;
  selected_index: number;
  correct_index?: number;
  selected_option_markdown?: string;
  correct_option_markdown?: string;
  essence_awarded?: number;
  essence?: number;
  remaining?: number;
};

type QuizOverlayMessage =
  | { type: 'quiz-result'; data: QuizResult }
  | { type: 'quiz-presented'; data: QuizPromptData };

type SearchParamsReader = {
  get(name: string): string | null;
};

type MathRendererWindow = Window & {
  renderMathInElement?: (
    element: HTMLElement,
    options: {
      delimiters: Array<{ left: string; right: string; display: boolean }>;
      throwOnError: boolean;
    }
  ) => void;
};

function parseOptions(value: string | null): string[] {
  try {
    const parsed: unknown = JSON.parse(value || '[]');
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((option): option is string => typeof option === 'string');
  } catch (e) {
    console.error(e);
    return [];
  }
}

function quizStateFromSearchParams(searchParams: SearchParamsReader): QuizState {
  return {
    quizId: searchParams.get('quiz_id') || '',
    question: searchParams.get('question') || '',
    options: parseOptions(searchParams.get('options')),
    type: searchParams.get('type') || 'mcq'
  };
}

function quizStateFromPrompt(promptData: QuizPromptData): QuizState {
  return {
    quizId: promptData.quiz_id,
    question: promptData.question_markdown,
    options: promptData.options_markdown || [],
    type: promptData.quiz_type === 'true_false' ? 'tf' : 'mcq'
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isQuizOverlayMessage(value: unknown): value is QuizOverlayMessage {
  if (!isRecord(value) || typeof value.type !== 'string' || !isRecord(value.data)) {
    return false;
  }
  return value.type === 'quiz-result' || value.type === 'quiz-presented';
}

function QuizOverlayContent() {
  const searchParams = useSearchParams();
  const isMobile = searchParams.get('mobile') === 'true';

  const [currentQuiz, setCurrentQuiz] = useState(() => quizStateFromSearchParams(searchParams));
  const currentQuizId = currentQuiz.quizId;
  const currentQuestion = currentQuiz.question;
  const currentOptions = currentQuiz.options;
  const currentType = currentQuiz.type;

  const [katexReady, setKatexReady] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [quizAnswered, setQuizAnswered] = useState(false);
  const [result, setResult] = useState<QuizResult | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Dynamically load KaTeX assets inside the iframe context
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

    // Register cross-document listener to receive results and next quiz presented events from Phaser
    const handleResultPacket = (event: MessageEvent) => {
      if (!isQuizOverlayMessage(event.data)) return;
      if (event.data.type === 'quiz-result') {
        setResult(event.data.data);
      } else if (event.data.type === 'quiz-presented') {
        setCurrentQuiz(quizStateFromPrompt(event.data.data));
        
        // Reset answer states for the new question
        setSelectedIndex(null);
        setQuizAnswered(false);
        setResult(null);
      }
    };
    window.addEventListener('message', handleResultPacket);

    return () => {
      window.removeEventListener('message', handleResultPacket);
      link.remove();
      scriptJs.remove();
    };
  }, []);

  // Run the KaTeX auto-typesetter when content or script is ready
  useEffect(() => {
    const mathWindow = window as MathRendererWindow;
    if (katexReady && containerRef.current && mathWindow.renderMathInElement) {
      try {
        mathWindow.renderMathInElement(containerRef.current, {
          delimiters: [
            { left: '$$', right: '$$', display: true },
            { left: '$', right: '$', display: false },
            { left: '\\(', right: '\\)', display: false },
            { left: '\\[', right: '\\]', display: true }
          ],
          throwOnError: false
        });
      } catch (e) {
        console.error('KaTeX typesetting crashed', e);
      }
    }
  }, [katexReady, currentQuestion, currentOptions]);

  const resultForCurrentQuiz = result && result.quiz_id === currentQuizId ? result : null;

  const optionHighlightClass = (index: number) => {
    if (!resultForCurrentQuiz) return '';
    if (selectedIndex === index) {
      return resultForCurrentQuiz.correct ? 'highlight-correct' : 'highlight-incorrect';
    }
    if (!resultForCurrentQuiz.correct && resultForCurrentQuiz.correct_index === index) {
      return 'highlight-correct';
    }
    return '';
  };

  const optionOpacity = (index: number) => {
    const isSelected = selectedIndex === index;
    const isCorrectAnswer = Boolean(
      resultForCurrentQuiz &&
      !resultForCurrentQuiz.correct &&
      resultForCurrentQuiz.correct_index === index
    );
    return quizAnswered && !isSelected && !isCorrectAnswer ? 0.5 : 1.0;
  };

  const handleSelectOption = (index: number) => {
    if (quizAnswered) return;
    setSelectedIndex(index);
    setQuizAnswered(true);
    // Post selected option back to the parent Phaser container
    window.parent.postMessage({ type: 'quiz-submit', index }, '*');
  };

  const handleClose = () => {
    window.parent.postMessage({ type: 'quiz-close' }, '*');
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

        /* Hide the moving sky clouds layout container inside this viewport */
        .sky-clouds {
          display: none !important;
        }

        /* Thinner mobile-only 9-slice outlines and borders */
        ${isMobile ? `
          .modal-9slice {
            border-width: 12px !important;
            border-image-width: 12px !important;
          }
          .question-9slice {
            border-width: 8px !important;
            border-image-width: 8px !important;
          }
          .option-9slice {
            border-width: 8px !important;
            border-image-width: 8px !important;
          }
        ` : ''}

        /* Dynamic premium 9-slice overlays utilizing vector SVGs */
        .modal-9slice {
          border: 24px solid transparent;
          border-image-source: url('/game/assets/gui/boxes_banners/Box_WhiteOutline_Square.svg');
          border-image-slice: 96 fill;
          border-image-width: 24px;
          border-image-repeat: stretch;
        }
        .question-9slice {
          border: 16px solid transparent;
          border-image-source: url('/game/assets/gui/boxes_banners/Box_Blank_Square.svg');
          border-image-slice: 64 fill;
          border-image-width: 16px;
          border-image-repeat: stretch;
        }
        .option-9slice {
          border: 12px solid transparent;
          border-image-source: url('/game/assets/gui/boxes_banners/Box_Blue_Square.svg');
          border-image-slice: 64 fill;
          border-image-width: 12px;
          border-image-repeat: stretch;
          transition: transform 0.15s ease, opacity 0.2s ease;
        }
        .btn-blue-simple {
          background-image: url('/game/assets/gui/buttons_text/ButtonText_Small_Blue_Round.svg');
          background-size: contain;
          background-position: center;
          background-repeat: no-repeat;
          transition: transform 0.15s ease, color 0.15s ease;
        }
        .btn-blue-simple:hover {
          color: #fef3c7 !important;
        }
        
        /* Interactive scaling hover actions */
        .option-hover:hover:not(.option-locked) {
          transform: scale(1.03);
          cursor: pointer;
        }
        .option-hover-tf:hover:not(.option-locked) {
          transform: scale(1.04);
          cursor: pointer;
        }
        .option-locked {
          cursor: default;
        }

        /* Tactical Hue-Rotate Color Highlights */
        .highlight-correct {
          filter: hue-rotate(260deg) saturate(1.8) brightness(1.2) !important;
        }
        .highlight-incorrect {
          filter: hue-rotate(140deg) saturate(2.0) brightness(1.2) !important;
          animation: shakeWrong 0.4s ease-in-out !important;
        }

        /* Tactical Shake Animation Keyframe for Incorrect Selections */
        @keyframes shakeWrong {
          0%, 100% { transform: translateX(0); }
          20%, 60% { transform: translateX(-8px); }
          40%, 80% { transform: translateX(8px); }
        }

        /* Custom solid circular loader spinner */
        .spinner {
          border: 6px solid rgba(255, 255, 255, 0.15);
          border-top: 6px solid #38bdf8;
          border-radius: 50%;
          width: 48px;
          height: 48px;
          animation: spin 0.8s linear infinite;
        }
        .button-spinner {
          border: 4px solid rgba(255, 255, 255, 0.15);
          border-top: 4px solid #38bdf8;
          border-radius: 50%;
          width: 24px;
          height: 24px;
          animation: spin 0.8s linear infinite;
        }
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }

        /* Float rise animation for correct answer rewards */
        @keyframes essenceFloat {
          0% {
            transform: translateY(30px) scale(0.7);
            opacity: 0;
          }
          15% {
            transform: translateY(0px) scale(1.15);
            opacity: 1;
          }
          85% {
            transform: translateY(-20px) scale(1.15);
            opacity: 1;
          }
          100% {
            transform: translateY(-50px) scale(0.9);
            opacity: 0;
          }
        }
        .animate-essence-float {
          animation: essenceFloat 2.0s forwards ease-in-out;
        }

        /* Stack math formulas vertically (below/above text) and center them */
        .katex {
          display: block !important;
          text-align: center !important;
          margin: 0.25em auto !important;
        }
        .katex-display {
          display: block !important;
          margin: 0.5em auto !important;
        }

        /* Premium fonts and layout rules */
        .font-concert {
          font-family: "Concert One", system-ui, sans-serif;
        }
        .scroll-hide::-webkit-scrollbar {
          display: none;
        }
        .scroll-hide {
          -ms-overflow-style: none;
          scrollbar-width: none;
        }
        .scroll-custom::-webkit-scrollbar {
          width: 6px;
          height: 6px;
          display: block;
        }
        .scroll-custom::-webkit-scrollbar-track {
          background: rgba(255, 255, 255, 0.05);
          border-radius: 4px;
        }
        .scroll-custom::-webkit-scrollbar-thumb {
          background: rgba(56, 189, 248, 0.35);
          border-radius: 4px;
        }
        .scroll-custom::-webkit-scrollbar-thumb:hover {
          background: rgba(56, 189, 248, 0.65);
        }
        .scroll-custom {
          scrollbar-width: thin;
          scrollbar-color: rgba(56, 189, 248, 0.35) rgba(255, 255, 255, 0.05);
        }

        /* Native responsive height flexbox layout */
        .modal-container {
          width: 96%;
          max-width: 580px; /* Cozy smaller width on mobile landscape */
          height: 96vh;
          height: 96dvh; /* Let the height grow fully to occupy screen vertically */
          display: flex;
          flex-direction: column;
          align-items: center;
          background: #0f172a;
          box-sizing: border-box;
          padding: 4px 12px 6px 12px !important; /* Even smaller paddings */
          overflow: hidden;
          position: relative;
        }

        .question-box {
          width: 95%;
          max-width: 540px; /* Cozy smaller width */
          height: 115px; /* Compact question card */
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          background: #020617;
          box-sizing: border-box;
          padding: 8px 14px; /* Cozy padding */
          font-size: 1.05rem; /* Smaller font text */
          margin-top: 4px;
        }

        .options-container {
          width: 95%;
          max-width: 540px; /* Cozy smaller width */
          margin-top: 4px; /* Reduced margin */
          padding: 2px; /* Even smaller paddings */
          box-sizing: border-box;
          overflow-y: auto;
          flex: 1; /* Dynamic flexible height! */
        }

        /* Large screen height style */
        @media (min-height: 600px) {
          .question-box {
            height: 240px;
            font-size: 1.55rem;
            padding: 20px;
          }
          .options-container {
            margin-top: 16px;
          }
        }

        @media (min-height: 750px) {
          .modal-container {
            height: 85vh;
            height: 85dvh;
            max-width: 800px; /* Restore desktop wider max-width if high resolution height is active */
          }
          .question-box {
            height: 320px;
            font-size: 1.75rem;
            padding: 24px;
            max-width: 680px;
          }
          .options-container {
            max-width: 680px;
          }
        }
      `}</style>

      <div 
        className={isMobile 
          ? "modal-9slice modal-container scroll-hide rounded-xl"
          : "modal-9slice relative flex flex-col items-center w-full max-w-[800px] max-h-[90vh] bg-slate-900 rounded-xl scroll-hide overflow-hidden"
        }
        style={isMobile 
          ? undefined 
          : { boxSizing: 'border-box', padding: '16px', paddingBottom: '32px' }
        }
      >
        {/* Close exit button */}
        <button 
          onClick={handleClose}
          className="absolute top-2 right-2 w-12 h-12 border-none bg-transparent cursor-pointer z-50 transition-transform duration-150 hover:scale-[1.15] active:scale-[0.95]"
          style={{
            backgroundImage: "url('/game/assets/gui/icons/Icon_Small_WhiteOutline_X.svg')",
            backgroundSize: 'contain',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat'
          }}
          aria-label="Close"
        />

        {/* Math Question Box */}
        <div 
          className={isMobile
            ? "question-9slice question-box font-concert text-center text-black select-text overflow-y-auto scroll-hide"
            : "question-9slice flex flex-col items-center justify-center w-[90%] max-w-[680px] h-[320px] bg-slate-950 font-concert text-2xl text-center text-black select-text overflow-y-auto scroll-hide mt-2"
          }
          style={isMobile
            ? undefined
            : { boxSizing: 'border-box', padding: '24px' }
          }
        >
          {currentQuestion}
        </div>

        {currentType === 'tf' ? (
          <div className={isMobile
            ? "options-container flex flex-row justify-center gap-6 p-2 scroll-custom"
            : "flex flex-row justify-center gap-6 w-full max-w-[680px] mt-6 p-2 max-h-[280px] overflow-y-auto scroll-custom"
          }>
            {currentOptions.map((option, idx) => {
              const isSelected = selectedIndex === idx;
              const highlightClass = optionHighlightClass(idx);
              return (
                <button
                  key={idx}
                  onClick={() => handleSelectOption(idx)}
                  disabled={quizAnswered}
                  className={`option-9slice option-hover-tf ${highlightClass} ${quizAnswered ? 'option-locked' : ''} flex items-center justify-center ${isMobile ? 'w-[45%] max-w-[280px] h-[75px]' : 'w-[42%] max-w-[280px] h-[140px]'} bg-transparent font-concert ${isMobile ? 'text-[15.5px]' : 'text-lg'} text-white text-center relative`}
                  style={{ 
                    boxSizing: 'border-box', 
                    padding: isMobile ? '6px 10px' : '20px', 
                    opacity: optionOpacity(idx)
                  }}
                >
                  <span className={quizAnswered && isSelected && !result ? "opacity-0 pointer-events-none" : "opacity-100"}>
                    {option}
                  </span>
                  {quizAnswered && isSelected && !result && (
                    <div className="absolute inset-0 flex items-center justify-center">
                      <div className="button-spinner" />
                    </div>
                  )}
                </button>
              );
            })}
          </div>
        ) : (
          <div className={isMobile
            ? "options-container flex flex-col items-center gap-3 p-2 scroll-custom"
            : "flex flex-col items-center gap-4 w-full mt-6 p-2 max-h-[280px] overflow-y-auto scroll-custom"
          }>
            {currentOptions.map((option, idx) => {
              const isSelected = selectedIndex === idx;
              const highlightClass = optionHighlightClass(idx);
              return (
                <button
                  key={idx}
                  onClick={() => handleSelectOption(idx)}
                  disabled={quizAnswered}
                  className={`option-9slice option-hover ${highlightClass} ${quizAnswered ? 'option-locked' : ''} flex flex-col items-start justify-center w-[90%] max-w-[680px] bg-transparent font-concert ${isMobile ? 'text-[15.5px]' : 'text-lg'} text-white text-left relative`}
                  style={{ 
                    boxSizing: 'border-box', 
                    padding: isMobile ? '8px 12px' : '16px', 
                    opacity: optionOpacity(idx)
                  }}
                >
                  <span className={quizAnswered && isSelected && !result ? "opacity-0 pointer-events-none" : "opacity-100"}>
                    {option}
                  </span>
                  {quizAnswered && isSelected && !result && (
                    <div className="absolute inset-0 flex items-center justify-center">
                      <div className="button-spinner" />
                    </div>
                  )}
                </button>
              );
            })}
          </div>
        )}

        {result && (
          <button 
            onClick={() => {
              window.parent.postMessage({ type: 'quiz-next' }, '*');
            }}
            className={isMobile 
              ? "btn-blue-simple mt-1 py-1.5 px-3 bg-transparent font-concert text-sm text-white text-center flex items-center justify-center cursor-pointer transition-transform duration-150 hover:scale-[1.05] active:scale-[0.95] border-none outline-none select-none z-50"
              : "btn-blue-simple mt-6 py-4 px-4 bg-transparent font-concert text-[20px] text-white text-center flex items-center justify-center cursor-pointer transition-transform duration-150 hover:scale-[1.05] active:scale-[0.95] border-none outline-none select-none z-50"
            }
            style={{ boxSizing: 'border-box' }}
          >
            <span style={{ textShadow: '0 2px 4px rgba(0,0,0,0.5)' }}>Next Quiz</span>
          </button>
        )}

        {result && result.correct && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-50 animate-essence-float font-concert text-5xl text-emerald-400 font-bold">
            +30 Essence
          </div>
        )}
      </div>
    </div>
  );
}

export default function QuizOverlayPage() {
  return (
    <Suspense fallback={
      <div className="fixed inset-0 w-full h-full flex items-center justify-center bg-transparent select-none font-sans text-white text-2xl">
        Loading Quiz Window...
      </div>
    }>
      <QuizOverlayContent />
    </Suspense>
  );
}
