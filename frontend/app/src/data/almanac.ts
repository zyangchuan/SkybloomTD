// alamanc for both enemies and birds



export interface AlmanacEntry {
    id: string;
    name: string;
    stats: Record<string, string | number>;
    evolveComb?: string
    desc: string;
}



export const BIRD_INFO: AlmanacEntry[] = [

     { id: 'sparrow', name: 'Sparrow', stats: { damage: 20, attack_range: 2.1, attack_speed: 1.0, attack_type: 'SINGLE', cost: 50 }, desc: 'Sparrow is the basic tower available to players. It provides balanced damage and attack_type speed at a low cost, making it suitable for the early stages of the game.' },
     { id: 'woodpecker', name: 'Woodpecker', stats: { damage: 10, attack_range: 2.1, attack_speed: 2.0, attack_type: 'SINGLE', cost: 65 }, desc: 'Woodpecker attacks faster than Sparrow but deals less damage per attack. It is effective against weaker enemies due to its higher attack speed.' },
     { id: 'eagle', name: 'Eagle', stats: { damage: 50, attack_range: 3.2, attack_speed: 0.5, attack_type: 'SINGLE', cost: 130 }, desc: 'Eagle deals high damage and has the longest attack range among the available bird towers. However, its slower attack speed makes it more suitable for targeting stronger enemies.' },
     { id: 'peacock',  name: 'Peacock', stats: { damage: 20, attack_range: 2.1, attack_speed: 1.0, attack_type: 'SPLASH', cost: 90 }, desc: 'Peacock specializes in area-of-effect attacks. It attacks enemies by firing 3 feather projectiles at once. This makes it particularly effective against groups of enemies travelling closely together.' },
     { id: 'falcon', name: 'Falcon', stats: { damage: 50, attack_range: 3.6, attack_speed: 0.9, attack_type: 'SINGLE', cost: 50 }, evolveComb: 'Sparrow + Eagle', desc: 'Falcon is a hybrid bird created by merging Sparrow and Eagle. It combines Sparrow’s balanced nature with Eagle’s stronger attack power, making it a powerful single-target tower.' },
     { id: 'kingfisher', name: 'Kingfisher', stats: { damage: 20, attack_range: 2.4, attack_speed: 3.0, attack_type: 'SPLASH', cost: 50 }, evolveComb: 'Woodpecker + Peacock', desc: 'Kingfisher is a hybrid bird created by merging Woodpecker and Peacock. It combines fast attack speed with splash damage, making it strong against groups of weaker enemies.' },
     { id: 'pheonix', name: 'Phoenix', stats: { damage: 25, attack_range: 3.0, attack_speed: 0.8, attack_type: 'SPLASH', cost: 50 }, evolveComb: 'Eagle + Peacock', desc: 'Phoenix is a hybrid bird created by merging Eagle and Peacock. It has strong damage and splash attack ability, making it effective against tougher enemy groups.' },
     { id: 'sun_god', name: 'Sun God', stats: { damage: 35, attack_range: 3.6, attack_speed: 1.6, attack_type: 'SPLASH', cost: 150 }, evolveComb: 'Kingfisher + Eagle', desc: 'Sun God is the strongest hybrid bird and is created by merging Eagle with Kingfisher. Since Kingfisher must first be created from Woodpecker and Peacock, Sun God requires multiple merge steps. It has high damage, strong range, fast attack speed, and splash damage, making it one of the most powerful towers in the game.' },

]