import type { ButtonHTMLAttributes } from 'react';
import styles from './ButtonOrange.module.css';

type ComponentProps = ButtonHTMLAttributes<HTMLButtonElement>;

export default function ButtonOrange({
  className,
  children,
  type = 'button',
  ...props
}: ComponentProps) {
  return (
    <button
      className={`${styles.buttonOrange} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-not-allowed disabled:opacity-75`}
      type={type}
      {...props}
    >
        {children}
    </button>
  )
}
