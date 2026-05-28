import React from 'react';
import { 
  Gamepad2, 
  Hourglass, 
  Loader2, 
  XCircle, 
  Trash2, 
  AlertTriangle 
} from 'lucide-react';
import GameCardBackground from './GameCardBackground';
import ButtonSmallGreenRound from './ButtonSmallGreenRound';

interface DocumentSummary {
  document_id: string;
  filename: string;
  game_name: string;
  is_ready: boolean;
  task_id: string;
  created_at: string;
  updated_at: string;
}

interface GameCardItemProps {
  doc: DocumentSummary;
  status: 'queued' | 'processing' | 'successful' | 'failed';
  errorMsg?: string | null;
  deletingDocId: string | null;
  setDeletingDocId: (id: string | null) => void;
  isDeletingInProgress: boolean;
  onDelete: (docId: string) => void;
  onPlay: (docId: string) => void;
}

export default function GameCardItem({
  doc,
  status,
  errorMsg,
  deletingDocId,
  setDeletingDocId,
  isDeletingInProgress,
  onDelete,
  onPlay,
}: GameCardItemProps) {
  const isConfirmingDelete = deletingDocId === doc.document_id;

  return (
    <GameCardBackground className="flex flex-col gap-2 transition-all duration-100 group">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
        <div className="flex flex-col items-start min-w-0">
          <span className="font-bold text-white text-md truncate w-full group-hover:text-yellow-200 transition-colors">
            {doc.game_name}
          </span>
          <span className="text-[10px] text-white/45 font-mono mt-0.5">
            Uploaded: {new Date(doc.created_at).toLocaleString()}
          </span>
        </div>

        <div className="flex items-center gap-3 self-start sm:self-auto shrink-0 select-none">
          {status === 'queued' && (
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-yellow-500/10 border border-yellow-500/30 text-yellow-300 text-xs font-semibold animate-pulse">
              <Hourglass className="w-3.5 h-3.5" /> Queued
            </div>
          )}
          {status === 'processing' && (
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-blue-500/10 border border-blue-500/30 text-blue-300 text-xs font-semibold">
              <Loader2 className="w-3.5 h-3.5 animate-spin" /> Processing
            </div>
          )}
          {status === 'successful' && (
            <ButtonSmallGreenRound
              onClick={() => onPlay(doc.document_id)}
              className="h-8 px-4 text-xs font-extrabold text-white active:scale-95 transition-all flex items-center gap-1.5 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
            >
              <Gamepad2 className="w-4 h-4 stroke-2 text-white" /> 
              <p className='text-white'>
                Play
              </p>
            </ButtonSmallGreenRound>
          )}
          {status === 'failed' && (
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-red-500/10 border border-red-500/30 text-red-300 text-xs font-semibold">
              <XCircle className="w-3.5 h-3.5" /> Failed
            </div>
          )}

          {!isConfirmingDelete && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setDeletingDocId(doc.document_id);
              }}
              className="w-8 h-8 flex items-center justify-center text-white hover:brightness-110 active:scale-95 transition-all cursor-pointer select-none bg-transparent animate-fadeIn shrink-0"
              style={{
                backgroundImage: "url('/gui/buttons_icons/IconButton_Small_Red_Circle.svg')",
                backgroundSize: '100% 100%',
                backgroundPosition: 'center',
                backgroundRepeat: 'no-repeat',
              }}
              title="Delete game and study notes"
            >
              <Trash2 className="w-4 h-4 text-white -mt-0.5" />
            </button>
          )}
        </div>
      </div>

      {status === 'failed' && errorMsg && (
        <div className="text-[10px] text-red-300 font-mono mt-1 bg-red-950/20 border border-red-900/30 rounded p-1.5 break-words">
          Reason: {errorMsg}
        </div>
      )}

      {isConfirmingDelete && (
        <div className="flex flex-col gap-2.5 p-3 bg-red-950/20 border border-red-500/30 rounded-lg mt-2">
          <div className="flex items-start gap-2">
            <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
            <p className="text-xs text-red-200 font-bold leading-relaxed">
              Warning: All generated game stages, quizzes, and levels from this document will be permanently deleted! This action is irreversible.
            </p>
          </div>
          <div className="flex gap-2 justify-end">
            <button
              type="button"
              onClick={() => setDeletingDocId(null)}
              className="px-3 py-1 bg-[#4a1900]/20 hover:bg-[#4a1900]/40 border border-yellow-800/20 text-[#fdfaf2] text-xs font-bold rounded transition-all active:scale-95 cursor-pointer"
              disabled={isDeletingInProgress}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => onDelete(doc.document_id)}
              className="px-3 py-1 bg-red-750 hover:bg-red-700 border-2 border-red-500 text-white text-xs font-bold rounded shadow-[0_2px_4px_rgba(239,68,68,0.2)] hover:shadow-[0_4px_8px_rgba(239,68,68,0.3)] transition-all active:scale-95 flex items-center gap-1 cursor-pointer disabled:cursor-not-allowed disabled:opacity-50"
              disabled={isDeletingInProgress}
            >
              {isDeletingInProgress ? (
                <>
                  <Loader2 className="w-3 h-3 animate-spin" /> Deleting...
                </>
              ) : (
                "Permanently Delete"
              )}
            </button>
          </div>
        </div>
      )}
    </GameCardBackground>
  );
}
