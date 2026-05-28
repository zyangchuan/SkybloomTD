import { useEffect, type ReactNode } from 'react';
import OrangeSquare from './OrangeSquare';
import styles from './Modal.module.css';

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  className?: string;
}

export default function Modal({
  isOpen,
  onClose,
  title,
  children,
  className = '',
}: ModalProps) {
  // Prevent background scrolling when modal is open
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  if (!isOpen) {
    return null;
  }

  return (
    <div className={styles.modalOverlay} onClick={onClose}>
      <div 
        className={`${styles.modalWrapper} w-full max-w-lg`}
        onClick={(e) => e.stopPropagation()}
      >
        <OrangeSquare className={`w-full flex flex-col px-6 py-4 text-center select-none relative ${className}`}>
          {/* Header */}
          <div className="flex items-center justify-between border-b-2 border-[#4a1900]/25 pb-3 mb-4 w-full">
            <h2 className="text-[#4a1900] text-2xl font-bold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
              {title}
            </h2>
            <button 
              className="text-[#4a1900]/80 hover:text-[#4a1900] text-2xl hover:scale-110 cursor-pointer font-bold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)] px-1 transition-all"
              onClick={onClose}
              aria-label="Close modal"
            >
              ✕
            </button>
          </div>
          
          {/* Body */}
          <div className="flex-1 w-full text-left">
            {children}
          </div>
        </OrangeSquare>
      </div>
    </div>
  );
}
