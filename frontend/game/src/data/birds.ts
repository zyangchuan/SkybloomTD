
// This file contains the stats for each bird type, including damage, range, fire rate, attack type, cost, and color.
export interface BirdStats {
  damage: number; 
  range: number; 
  fireRate: string; 
  attack: string; 
  cost: number; 
}

export const BIRD_STATS: Record<string, BirdStats> = {
  sparrow:    { damage: 10, range: 3.5, fireRate: '1.0/s', attack: 'SINGLE', cost:  50 },
  woodpecker: { damage:  6, range: 3.5, fireRate: '2.0/s', attack: 'SINGLE', cost:  65 },
  eagle:      { damage: 30, range: 6.0, fireRate: '0.4/s', attack: 'SINGLE', cost: 130 },
  peacock:    { damage:  7, range: 3.5, fireRate: '1.0/s', attack: 'SPLASH', cost:  90 },
};

export const EVOLVED_BIRD_STATS: Record<string, BirdStats> = {
  sparrow:    { damage: 18, range: 4.5, fireRate: '1.5/s', attack: 'SINGLE', cost:  50 },
  woodpecker: { damage: 11, range: 4.5, fireRate: '3.0/s', attack: 'SINGLE', cost:  65 },
  eagle:      { damage: 50, range: 7.5, fireRate: '0.6/s', attack: 'SINGLE', cost: 130 },
  peacock:    { damage: 13, range: 4.5, fireRate: '1.5/s', attack: 'SPLASH', cost:  90 },
};

export const DAMAGE_TO_BIRD: Record<number, string> = {
   6: 'woodpecker',   30: 'eagle',   7: 'peacock', 10 : 'sparrow',
  11: 'evolve_woodpecker', 50: 'evolve_eagle', 13: 'evolve_peacock', 18: 'evolve_sparrow',
};
