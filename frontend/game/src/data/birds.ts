export const BIRD_STATS: Record<string, {
  damage: number, range: number, pierce?: number, spread?: number, fireRate: string, attack: string, cost: number, color: string
}> = {
  sparrow:    { damage: 20, range: 2.1, fireRate: '1.0/s', attack: 'SINGLE', cost:  50, color: '#38bdf8' },
  woodpecker: { damage: 10, range: 2.1, fireRate: '2.0/s', attack: 'SINGLE', cost:  65, color: '#fb7185' },
  eagle:      { damage: 50, range: 3.2, fireRate: '0.5/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
  peacock:    { damage: 20, range: 2.1, spread: Math.PI / 12, fireRate: '1.0/s', attack: 'SPLASH', cost:  90, color: '#c084fc' },
  falcon:     { damage: 50, range: 3.6, fireRate: '0.9/s', attack: 'SINGLE', cost:  50, color: '#f97316' },
  kingfisher: { damage: 20, range: 2.4, spread: Math.PI / 12, fireRate: '3.0/s', attack: 'SPLASH', cost:  50, color: '#06b6d4' },
  phoenix:    { damage: 25, range: 2.0, fireRate: '0.8/s', attack: 'RING', cost:  50, color: '#ef4444' },
  sun_god:    { damage: 18, range: 4.5, pierce: 5, spread: Math.PI / 36, fireRate: '3.2/s', attack: 'SPLASH', cost: 150, color: '#eab308' },
};
