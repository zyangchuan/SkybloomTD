import styles from './BlueSquare.module.css';

interface ComponentProps {
  className?: string; 
  children?: React.ReactNode; 
}

export default function BlueSquare({ className, children }: ComponentProps) {
  return (
    <div className={`${styles.blueSquare} ${className || ''}`}>
        {children}
    </div>
  )
}
