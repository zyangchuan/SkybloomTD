import styles from './OrangeSquare.module.css';

interface ComponentProps {
  className?: string; 
  children?: React.ReactNode; 
}

export default function OrangeSquare({ className, children }: ComponentProps) {
  return (
    <div className={`${styles.orangeSquare} ${className || ''}`}>
        {children}
    </div>
  )
}

