import React, { useState } from 'react';
import { 
  Gamepad2, 
  Trash2, 
  AlertTriangle,
  Loader2,
  Star
} from 'lucide-react';
import GameCardBackground from './GameCardBackground';
import ButtonSmallGreenRound from './ButtonSmallGreenRound';
import VisibilityToggle from './VisibilityToggle';

export interface GameLibrarySummary {
  document_id: string;
  user_id: string;
  source_filename: string;
  game_name: string;
  is_ready: boolean;
  is_public: boolean;
  starred_by_me: boolean;
  created_at: string;
  updated_at: string;
}

interface LibraryGameCardProps {
  game: GameLibrarySummary;
  currentUserId: string;
  onToggleStar: (documentId: string, currentStarred: boolean) => void;
  onPlay: (documentId: string) => void;
  onToggleVisibility?: (documentId: string, currentPublic: boolean) => void;
  onDelete?: (documentId: string) => void;
  isDeletingInProgress?: boolean;
}

export default function LibraryGameCard({
  game,
  currentUserId,
  onToggleStar,
  onPlay,
  onToggleVisibility,
  onDelete,
  isDeletingInProgress = false,
}: LibraryGameCardProps) {
  const [isConfirmingDelete, setIsConfirmingDelete] = useState(false);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const isOwner = game.user_id === currentUserId;

  return (
    <GameCardBackground className="flex flex-col gap-2 transition-all duration-100 group">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
        <div className="flex flex-col items-start min-w-0">
          <span className="font-bold text-white text-lg truncate w-full group-hover:text-yellow-200 transition-colors">
            {game.game_name}
          </span>
          <span className="text-[12px] text-white/45 font-mono mt-0.5 flex gap-2 items-center flex-wrap">
            <span>Uploaded: {new Date(game.created_at).toLocaleDateString()}</span>
            {isOwner ? (
              <span className="px-1.5 py-0.5 rounded bg-yellow-500/10 border border-yellow-500/30 text-yellow-300 text-[10px] font-bold">
                Owned
              </span>
            ) : (
              <span className="px-1.5 py-0.5 rounded bg-blue-500/10 border border-blue-500/30 text-blue-300 text-[10px] font-bold">
                Shared
              </span>
            )}
          </span>
        </div>

        <div className="flex items-center gap-2.5 self-start sm:self-auto shrink-0 select-none relative">
          {/* Star Button */}
          {!isOwner && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onToggleStar(game.document_id, game.starred_by_me);
              }}
              className="w-8 h-8 flex items-center justify-center text-yellow-400 active:scale-95 transition-all cursor-pointer bg-transparent border-0 shrink-0 group/star"
              title={game.starred_by_me ? 'Unstar game' : 'Star game'}
            >
              {game.starred_by_me ? (
                <img
                  src="/gui/icons/Icon_Small_Star.svg"
                  alt="Starred"
                  className="w-5.5 h-5.5 transition-transform duration-100 group-hover/star:scale-110"
                />
              ) : (
                <Star
                  className="w-5.5 h-5.5 text-white/40 hover:text-yellow-300 transition-all duration-100 group-hover/star:scale-110"
                />
              )}
            </button>
          )}

          {/* Play Button */}
          <ButtonSmallGreenRound
            onClick={() => onPlay(game.document_id)}
            className="h-8 px-3.5 text-xs font-extrabold text-white active:scale-95 transition-all flex items-center gap-1.5 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
          >
            <Gamepad2 className="w-4 h-4 stroke-2 text-white" />
            <span className="text-white">Play</span>
          </ButtonSmallGreenRound>

          {/* Collapsed Options Menu Button (Owner Only, only when not confirming delete) */}
          {isOwner && (onToggleVisibility || onDelete) && !isConfirmingDelete && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setIsMenuOpen(!isMenuOpen);
              }}
              className="cursor-pointer"
              title="Options"
            >
              <img
                src="/gui/icons/Icon_Small_Blank_Menu.svg"
                alt="Options"
                className="w-6 h-6"
              />
            </button>
          )}
        </div>
      </div>

      {/* Inline Options Menu */}
      {isOwner && (onToggleVisibility || onDelete) && !isConfirmingDelete && isMenuOpen && (
        <div className="flex items-center justify-between gap-4 p-3 bg-[#4a1900]/10 border border-yellow-600/20 rounded-lg mt-2 animate-fadeIn">
          <div className="flex-1">
            {onToggleVisibility && (
              <VisibilityToggle
                isPublic={game.is_public}
                onToggle={() => {
                  onToggleVisibility(game.document_id, game.is_public);
                }}
              />
            )}
          </div>
          {onDelete && (
            <button
              type="button"
              onClick={() => {
                setIsConfirmingDelete(true);
                setIsMenuOpen(false);
              }}
              className="flex items-center gap-1 font-bold text-white text-xs hover:brightness-110 active:scale-95 transition-all cursor-pointer select-none bg-transparent"
              style={{
                border: '8px solid transparent',
                borderImageSource: "url('/gui/buttons_icons/IconButton_Small_Red_Square.svg')",
                borderImageSlice: '80 fill',
                borderImageRepeat: 'stretch',
              }}
            >
              <Trash2 className="w-3.5 h-3.5 text-white -mt-0.5" />
              <span>Delete Game</span>
            </button>
          )}
        </div>
      )}

      {/* Delete confirmation modal UI inside card */}
      {isOwner && onDelete && isConfirmingDelete && (
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
              onClick={() => setIsConfirmingDelete(false)}
              className="px-3 py-1 bg-[#4a1900]/20 hover:bg-[#4a1900]/40 border border-yellow-800/20 text-[#fdfaf2] text-xs font-bold rounded transition-all active:scale-95 cursor-pointer"
              disabled={isDeletingInProgress}
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => {
                onDelete(game.document_id);
                setIsConfirmingDelete(false);
              }}
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
