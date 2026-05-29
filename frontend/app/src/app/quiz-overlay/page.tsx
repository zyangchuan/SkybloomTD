'use client';

import React, { Suspense, useEffect, useState, useRef } from 'react';
import { useSearchParams } from 'next/navigation';

function QuizOverlayContent() {
  const searchParams = useSearchParams();
  const quizId = searchParams.get('quiz_id') || '';
  const rawQuestion = searchParams.get('question') || '';
  const rawOptions = searchParams.get('options') || '[]';
  const type = searchParams.get('type') || 'mcq';

  let optionsList: string[] = [];
  try {
    optionsList = JSON.parse(rawOptions);
  } catch (e) {
    console.error('Failed to parse options in quiz overlay', e);
  }

  const [katexReady, setKatexReady] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [quizAnswered, setQuizAnswered] = useState(false);
  const [result, setResult] = useState<any | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Dynamically load KaTeX assets inside the iframe context
  useEffect(() => {
    // 1. Preload stylesheet
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/katex.min.css';
    document.head.appendChild(link);

    // 2. Preload KaTeX core parser
    const scriptJs = document.createElement('script');
    scriptJs.src = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/katex.min.js';
    scriptJs.async = false;
    scriptJs.onload = () => {
      // 3. Preload auto-render extension
      const scriptAuto = document.createElement('script');
      scriptAuto.src = 'https://cdn.jsdelivr.net/npm/katex@0.16.8/dist/contrib/auto-render.min.js';
      scriptAuto.async = false;
      scriptAuto.onload = () => {
        setKatexReady(true);
      };
      document.head.appendChild(scriptAuto);
    };
    document.head.appendChild(scriptJs);

    // 4. Register cross-document listener to receive results from Phaser
    const handleResultPacket = (event: MessageEvent) => {
      if (event.data.type === 'quiz-result') {
        setResult(event.data.data);
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
    if (katexReady && containerRef.current && (window as any).renderMathInElement) {
      try {
        (window as any).renderMathInElement(containerRef.current, {
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
  }, [katexReady, rawQuestion, rawOptions]);

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
      className="fixed inset-0 w-full h-full flex items-center justify-center p-4 bg-slate-950/75 select-none overflow-hidden"
    >
      <style>{`
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
          transition: transform 0.15s ease, filter 0.25s ease, opacity 0.2s ease;
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
          filter: hue-rotate(260deg) saturate(1.8) brightness(1.2) drop-shadow(0 0 16px #22c55e) !important;
          animation: correctPulse 1.2s ease-in-out infinite !important;
        }
        .highlight-incorrect {
          filter: hue-rotate(140deg) saturate(2.0) brightness(1.2) drop-shadow(0 0 16px #ef4444) !important;
          animation: incorrectPulse 1.2s ease-in-out infinite !important;
        }

        /* Pulsing Glow Animation Keyframes */
        @keyframes correctPulse {
          0% { transform: scale(1.0); filter: hue-rotate(260deg) saturate(1.8) brightness(1.2) drop-shadow(0 0 8px rgba(34, 197, 94, 0.6)); }
          50% { transform: scale(1.03); filter: hue-rotate(260deg) saturate(1.8) brightness(1.2) drop-shadow(0 0 24px rgba(34, 197, 94, 0.95)); }
          100% { transform: scale(1.0); filter: hue-rotate(260deg) saturate(1.8) brightness(1.2) drop-shadow(0 0 8px rgba(34, 197, 94, 0.6)); }
        }
        @keyframes incorrectPulse {
          0% { transform: scale(1.0); filter: hue-rotate(140deg) saturate(2.0) brightness(1.2) drop-shadow(0 0 8px rgba(239, 68, 68, 0.6)); }
          50% { transform: scale(1.03); filter: hue-rotate(140deg) saturate(2.0) brightness(1.2) drop-shadow(0 0 24px rgba(239, 68, 68, 0.95)); }
          100% { transform: scale(1.0); filter: hue-rotate(140deg) saturate(2.0) brightness(1.2) drop-shadow(0 0 8px rgba(239, 68, 68, 0.6)); }
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
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
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
      `}</style>

      {/* Main modal container */}
      <div 
        className="modal-9slice relative flex flex-col items-center w-full max-w-[800px] max-h-[90vh] bg-slate-900 rounded-xl scroll-hide overflow-y-auto"
        style={{ boxSizing: 'border-box', padding: '16px' }}
      >
        {/* Close exit button */}
        <button 
          onClick={handleClose}
          className="absolute top-2 right-2 w-12 h-12 border-none bg-transparent cursor-pointer z-50 transition-transform duration-150 hover:scale-[1.15] active:scale-[0.95]"
          style={{
            backgroundImage: "url('/game/assets/gui/buttons_text/PremadeButtons_ExitOrange.svg')",
            backgroundSize: 'contain',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat'
          }}
          aria-label="Close"
        />

        {/* Math Question Box */}
        <div 
          className="question-9slice flex items-center justify-center w-[90%] max-w-[680px] h-[320px] bg-slate-950 font-concert text-3xl text-center text-white select-text overflow-y-auto scroll-hide mt-8"
          style={{ boxSizing: 'border-box', padding: '24px' }}
        >
          {rawQuestion}
        </div>

        {/* Dynamic MCQ / TF Selection layout */}
        {type === 'tf' ? (
          <div className="flex flex-row justify-center gap-6 w-full max-w-[680px] mt-6">
            {optionsList.map((option, idx) => {
              const isSelected = selectedIndex === idx;
              let highlightClass = '';
              if (isSelected && result) {
                highlightClass = result.correct ? 'highlight-correct' : 'highlight-incorrect';
              }
              return (
                <button
                  key={idx}
                  onClick={() => handleSelectOption(idx)}
                  disabled={quizAnswered}
                  className={`option-9slice option-hover-tf ${highlightClass} ${quizAnswered ? 'option-locked' : ''} flex items-center justify-center w-[42%] max-w-[280px] h-[140px] bg-slate-900 font-concert text-2xl text-white text-center`}
                  style={{ 
                    boxSizing: 'border-box', 
                    padding: '20px', 
                    opacity: quizAnswered && !isSelected ? 0.5 : 1.0 
                  }}
                >
                  {option}
                </button>
              );
            })}
          </div>
        ) : (
          <div className="flex flex-col items-center gap-4 w-full mt-6">
            {optionsList.map((option, idx) => {
              const isSelected = selectedIndex === idx;
              let highlightClass = '';
              if (isSelected && result) {
                highlightClass = result.correct ? 'highlight-correct' : 'highlight-incorrect';
              }
              return (
                <button
                  key={idx}
                  onClick={() => handleSelectOption(idx)}
                  disabled={quizAnswered}
                  className={`option-9slice option-hover ${highlightClass} ${quizAnswered ? 'option-locked' : ''} flex items-center justify-center w-[90%] max-w-[680px] h-[80px] bg-slate-900 font-concert text-2xl text-white text-center`}
                  style={{ 
                    boxSizing: 'border-box', 
                    padding: '16px', 
                    opacity: quizAnswered && !isSelected ? 0.5 : 1.0 
                  }}
                >
                  {option}
                </button>
              );
            })}
          </div>
        )}

        {/* Feedback Bottom area */}
        <div 
          className="flex items-center justify-center w-full h-[120px] font-concert text-2xl text-center select-text z-10"
          style={{ paddingBottom: '40px', boxSizing: 'border-box' }}
        >
          {quizAnswered && !result && (
            <div className="spinner" />
          )}
          {result && (
            <div 
              style={{
                color: result.correct ? '#22c55e' : '#ef4444',
                textShadow: result.correct 
                  ? '0 0 10px rgba(34, 197, 94, 0.4)' 
                  : '0 0 10px rgba(239, 68, 68, 0.4)'
              }}
            >
              {result.feedback || (result.correct ? 'CORRECT!' : 'INCORRECT!')}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default function QuizOverlayPage() {
  return (
    <Suspense fallback={
      <div className="fixed inset-0 w-full h-full flex items-center justify-center bg-slate-950/75 select-none font-sans text-white text-2xl">
        Loading Quiz Window...
      </div>
    }>
      <QuizOverlayContent />
    </Suspense>
  );
}
