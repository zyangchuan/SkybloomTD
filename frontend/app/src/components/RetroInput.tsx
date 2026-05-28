import type { InputHTMLAttributes } from 'react';
import styles from './RetroInput.module.css';

interface CustomProps {
  label?: string;
}

type ComponentProps = InputHTMLAttributes<HTMLInputElement> & CustomProps;

export default function RetroInput({
  className,
  label,
  id,
  ...props
}: ComponentProps) {
  return (
    <div className="flex flex-col w-full gap-1">
      {label && (
        <label 
          htmlFor={id} 
          className="text-[#4a1900] text-lg ml-2 drop-shadow-[0_1px_0_rgba(255,255,255,0.4)] text-left font-bold"
        >
          {label}
        </label>
      )}
      <div className={`${styles.retroInputContainer} ${className || ''}`}>
        <input
          id={id}
          className="w-full bg-transparent border-0 outline-none text-black text-lg px-2 placeholder:text-black/40 font-sans"
          {...props}
        />
      </div>
    </div>
  )
}
