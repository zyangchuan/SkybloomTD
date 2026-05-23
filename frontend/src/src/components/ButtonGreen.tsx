import type { ButtonHTMLAttributes } from 'react';
import styles from './ButtonGreen.module.css';

type ComponentProps = ButtonHTMLAttributes<HTMLButtonElement>;

export default function ButtonGreen({
  className,
  children,
  type = 'button',
  ...props
}: ComponentProps) {
  return (
    <button
      className={`${styles.buttonGreen} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-wait disabled:opacity-75`}
      type={type}
      {...props}
    >
        {children}
    </button>
  )
}
