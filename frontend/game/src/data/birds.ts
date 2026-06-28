export const BIRD_STATS: Record<string, {
  damage: number, range: number, fireRate: string, attack: string, cost: number, color: string
}> = {
  sparrow:    { damage: 10, range: 2.1, fireRate: '1.0/s', attack: 'SINGLE', cost:  50, color: '#38bdf8' },
  woodpecker: { damage:  6, range: 2.1, fireRate: '2.0/s', attack: 'SINGLE', cost:  65, color: '#fb7185' },
  eagle:      { damage: 30, range: 3.2, fireRate: '0.4/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
  peacock:    { damage:  7, range: 2.1, fireRate: '1.0/s', attack: 'SPLASH', cost:  90, color: '#c084fc' },
  falcon:     { damage: 24, range: 3.6, fireRate: '0.9/s', attack: 'SINGLE', cost:   0, color: '#f97316' },
  kingfisher: { damage:  9, range: 2.4, fireRate: '3.0/s', attack: 'SPLASH', cost:   0, color: '#06b6d4' },
  phoenix:    { damage: 28, range: 3.0, fireRate: '0.8/s', attack: 'SPLASH', cost:   0, color: '#ef4444' },
  sun_god:    { damage: 32, range: 3.6, fireRate: '1.6/s', attack: 'SPLASH', cost:   0, color: '#eab308' },
};

export const DAMAGE_TO_BIRD: Record<number, string> = {
  10: 'sparrow',
   6: 'woodpecker',
  30: 'eagle',
   7: 'peacock',
  24: 'falcon',
   9: 'kingfisher',
  28: 'phoenix',
  32: 'sun_god',
};
