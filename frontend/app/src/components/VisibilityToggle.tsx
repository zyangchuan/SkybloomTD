import React from 'react';

interface VisibilityToggleProps {
  isPublic: boolean;
  onToggle: () => void;
  disabled?: boolean;
}

export default function VisibilityToggle({ isPublic, onToggle, disabled = false }: VisibilityToggleProps) {
  return (
    <div className="flex items-center gap-2 select-none shrink-0">
      <span className="text-[15px] text-white font-bold tracking-wider min-w-[60px] text-right">
        {isPublic ? 'Public' : 'Private'}
      </span>
      <button
        type="button"
        disabled={disabled}
        onClick={(e) => {
          e.stopPropagation();
          onToggle();
        }}
        className={`w-14 h-8 relative flex items-center cursor-pointer transition-all duration-200 select-none bg-transparent border-0 outline-none ${
          disabled ? 'opacity-50 cursor-not-allowed' : 'hover:brightness-110 active:scale-95'
        }`}
        style={{
          backgroundImage: `url(${
            isPublic 
              ? '/gui/buttons_text/ButtonText_Blue_OnOffBackground.svg' 
              : '/gui/buttons_text/ButtonText_Background_OnOffBackground.svg'
          })`,
          backgroundSize: '100% 100%',
          backgroundPosition: 'center',
          backgroundRepeat: 'no-repeat',
        }}
        title={isPublic ? 'Change to Private' : 'Change to Public'}
      >
        {/* Slider Knob */}
        <div
          className="transition-transform duration-200 ease-out"
          style={{
            width: '25px',
            height: '25px',
            backgroundImage: "url('/gui/buttons_text/ButtonText_Blank_OnOffButton.svg')",
            backgroundSize: '100% 100%',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat',
            transform: isPublic ? 'translateX(31px)' : 'translateX(0px)',
          }}
        />
      </button>
    </div>
  );
}
