export const BIRD_STATS: Record<string, {
  damage: number, range: number, fireRate: string, attack: string, cost: number, color: string
}> = {
  sparrow:    { damage: 10, range: 3.5, fireRate: '1.0/s', attack: 'SINGLE', cost:  50, color: '#38bdf8' },
  woodpecker: { damage:  6, range: 3.5, fireRate: '2.0/s', attack: 'SINGLE', cost:  65, color: '#fb7185' },
  eagle:      { damage: 30, range: 6.0, fireRate: '0.4/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
  peacock:    { damage:  7, range: 3.5, fireRate: '1.0/s', attack: 'SPLASH', cost:  90, color: '#c084fc' },
};

export const DAMAGE_TO_BIRD: Record<number, string> = {
   6: 'woodpecker',
  30: 'eagle',
   7: 'peacock',
};
