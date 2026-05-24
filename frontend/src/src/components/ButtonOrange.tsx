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
<<<<<<< HEAD
      className={`${styles.buttonOrange} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-wait disabled:opacity-75`}
=======
      className={`${styles.buttonOrange} ${className || ''} flex cursor-pointer items-center justify-center disabled:cursor-not-allowed disabled:opacity-75`}
>>>>>>> d9013ee (added upload document feature)
      type={type}
      {...props}
    >
        {children}
    </button>
  )
}
