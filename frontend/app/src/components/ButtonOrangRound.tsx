import type { ButtonHTMLAttributes } from 'react';
import styles from './ButtonOrangeRound.module.css';

type ComponentProps = ButtonHTMLAttributes<HTMLButtonElement>;

export default function ButtonOrangeRound({
  className,
  children,
  type = 'button',
  ...props
}: ComponentProps) {
  return (
    <button
      className={`${styles.buttonOrangeRound} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-not-allowed disabled:opacity-75`}
      type={type}
      {...props}
    >
        {children}
    </button>
  )
}
