export const BIRD_STATS: Record<string, {
  damage: number, range: number, lifespan?: number, spread?: number, fireRate: string, attack: string, cost: number, color: string
}> = {
  sparrow:    { damage: 20, range: 2.1, lifespan: 2.1, fireRate: '1.0/s', attack: 'SINGLE', cost:  50, color: '#38bdf8' },
  woodpecker: { damage: 10, range: 2.1, lifespan: 2.1, fireRate: '2.0/s', attack: 'SINGLE', cost:  65, color: '#fb7185' },
  eagle:      { damage: 50, range: 3.2, lifespan: 3.2, fireRate: '0.5/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
  peacock:    { damage: 20, range: 2.1, lifespan: 2.1, spread: Math.PI / 12, fireRate: '1.0/s', attack: 'SPLASH', cost:  90, color: '#c084fc' },
  falcon:     { damage: 50, range: 3.6, lifespan: 3.6, fireRate: '0.9/s', attack: 'SINGLE', cost:  50, color: '#f97316' },
  kingfisher: { damage: 20, range: 2.4, lifespan: 2.4, spread: Math.PI / 12, fireRate: '3.0/s', attack: 'SPLASH', cost:  50, color: '#06b6d4' },
  phoenix:    { damage: 25, range: 2.0, fireRate: '0.8/s', attack: 'RING', cost:  50, color: '#ef4444' },
  sun_god:    { damage: 35, range: 3.6, lifespan: 20.0, spread: Math.PI / 18, fireRate: '1.6/s', attack: 'SPLASH', cost: 150, color: '#eab308' },
};
