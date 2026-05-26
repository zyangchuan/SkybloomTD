import type { ButtonHTMLAttributes } from 'react';
import styles from './ButtonSmallGreenRound.module.css';

type ComponentProps = ButtonHTMLAttributes<HTMLButtonElement>;

export default function ButtonSmallGreenRound({
  className,
  children,
  type = 'button',
  ...props
}: ComponentProps) {
  return (
    <button
      className={`${styles.buttonSmallGreenRound} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-not-allowed disabled:opacity-75`}
      type={type}
      {...props}
    >
        {children}
    </button>
  );
}
