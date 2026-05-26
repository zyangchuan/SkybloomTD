import React from 'react';

interface ChapterSpriteProps {
  index: number;
  className?: string;
  size?: number; // width and height in px
}

export default function ChapterSprite({ index, className = '', size = 128 }: ChapterSpriteProps) {
  const columns = 3;
  const rows = 2;
  const totalSprites = columns * rows; // 6
  
  // Wrap index to repeat if index is higher than the total unique sprites available
  const spriteIndex = index >= 0 ? index % totalSprites : 0;
  
  const col = spriteIndex % columns;
  const row = Math.floor(spriteIndex / columns);
  
  // Percentage offset logic for background-position
  const posX = columns > 1 ? (col / (columns - 1)) * 100 : 0;
  const posY = rows > 1 ? (row / (rows - 1)) * 100 : 0;
  
  return (
    <div
      className={`select-none shrink-0 pointer-events-none ${className}`}
      style={{
        width: `${size}px`,
        height: `${size}px`,
        backgroundImage: "url('/assets/chapter_sprite.png')",
        backgroundSize: `${columns * 100}% ${rows * 100}%`,
        backgroundPosition: `${posX}% ${posY}%`,
        backgroundRepeat: 'no-repeat',
      }}
    />
  );
}
