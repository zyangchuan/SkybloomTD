import React, { HTMLAttributes } from 'react';
import styles from './GameCardBackground.module.css';

type ComponentProps = HTMLAttributes<HTMLDivElement>;

export default function GameCardBackground({
  className,
  children,
  ...props
}: ComponentProps) {
  return (
    <div
      className={`${styles.gameCardBackground} ${className || ''}`}
      {...props}
    >
      {children}
    </div>
  );
}
