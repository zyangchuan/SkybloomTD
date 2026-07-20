// alamanc for both enemies and birds

export interface AlmanacBirdsEntry {
    id: string;
    name: string;
    stats: Record<string, string | number>;
    evolveComb?: string
    desc: string;
}

export interface AlmanacEnemiesEntry {
    id: string;
    name: string;
    stats: Record<string, number | string>;
    desc: string;
}

export const BIRD_INFO: AlmanacBirdsEntry[] = [

    { id: 'sparrow', name: 'Sparrow', stats: { damage: 20, attack_range: 2.1, attack_speed: 1.0, attack_type: 'SINGLE', cost: 50 }, desc: 'Sparrow is the basic tower available to players. It provides balanced damage and attack_type speed at a low cost, making it suitable for the early stages of the game.' },
    { id: 'woodpecker', name: 'Woodpecker', stats: { damage: 10, attack_range: 2.1, attack_speed: 2.0, attack_type: 'SINGLE', cost: 65 }, desc: 'Woodpecker attacks faster than Sparrow but deals less damage per attack. It is effective against weaker enemies due to its higher attack speed.' },
    { id: 'eagle', name: 'Eagle', stats: { damage: 50, attack_range: 3.2, attack_speed: 0.5, attack_type: 'SINGLE', cost: 130 }, desc: 'Eagle deals high damage and has the longest attack range among the available bird towers. However, its slower attack speed makes it more suitable for targeting stronger enemies.' },
    { id: 'peacock', name: 'Peacock', stats: { damage: 20, attack_range: 2.1, attack_speed: 1.0, attack_type: 'SPLASH', cost: 90 }, desc: 'Peacock specializes in area-of-effect attacks. It attacks enemies by firing 3 feather projectiles at once. This makes it particularly effective against groups of enemies travelling closely together.' },
    { id: 'falcon', name: 'Falcon', stats: { damage: 50, attack_range: 3.6, attack_speed: 0.9, attack_type: 'SINGLE', cost: 50 }, evolveComb: 'Sparrow + Eagle', desc: 'Falcon is a hybrid bird created by merging Sparrow and Eagle. It combines Sparrow’s balanced nature with Eagle’s stronger attack power, making it a powerful single-target tower.' },
    { id: 'kingfisher', name: 'Kingfisher', stats: { damage: 20, attack_range: 2.4, attack_speed: 3.0, attack_type: 'SPLASH', cost: 50 }, evolveComb: 'Woodpecker + Peacock', desc: 'Kingfisher is a hybrid bird created by merging Woodpecker and Peacock. It combines fast attack speed with splash damage, making it strong against groups of weaker enemies.' },
    { id: 'pheonix', name: 'Phoenix', stats: { damage: 25, attack_range: 3.0, attack_speed: 0.8, attack_type: 'SPLASH', cost: 50 }, evolveComb: 'Eagle + Peacock', desc: 'Phoenix is a hybrid bird created by merging Eagle and Peacock. It has strong damage and splash attack ability, making it effective against tougher enemy groups.' },
    { id: 'sun_god', name: 'Sun God', stats: { damage: 35, attack_range: 3.6, attack_speed: 1.6, attack_type: 'SPLASH', cost: 150 }, evolveComb: 'Kingfisher + Eagle', desc: 'Sun God is the strongest hybrid bird and is created by merging Eagle with Kingfisher. Since Kingfisher must first be created from Woodpecker and Peacock, Sun God requires multiple merge steps. It has high damage, strong range, fast attack speed, and splash damage, making it one of the most powerful towers in the game.' },

]

export const ENEMY_INFO: AlmanacEnemiesEntry[] = [
    
    { id: "smog", name: 'Smog', stats: { health: 100, movement: 1.0 }, desc: 'Smog is the primary enemy in the game and represents air pollution threatening the environment. Smog moves steadily along the designated path toward the player\'s base. Players must place bird towers strategically to eliminate Smog before it reaches its destination. As the wave number increases, Smog gains higher health and movement speed.' },
    { id: "junk", name: 'Junk', stats: { health: 150, movement: 0.8, }, desc: 'Noise is a fast-moving enemy that represents noise pollution. Compared to Smog and Junk, Noise has lower health but moves much faster along the path. Its speed makes it dangerous because it can quickly reach the player\'s base if not targeted early. Players must react quickly and use well-positioned bird towers to stop Noise in time. As the wave number increases, Noise becomes faster and slightly tougher.' },
    { id: "noise", name: 'Noise', stats: { health: 80, movement: 1.4, }, desc: 'Junk is a tougher enemy that represents physical waste and environmental pollution. Junk has much higher health than other enemies, making it harder to defeat quickly. Although it moves slowly, its durability allows it to survive longer on the path. Junk appears in later waves to increase the difficulty and encourage players to use stronger or upgraded bird towers.' }

]

