import Phaser from 'phaser';
import { BIRD_STATS } from '../data/birds';


export class BirdTray {
  constructor(scene: Phaser.Scene, isDragging: () => boolean/*, isEvolved: (birdType: string) => boolean = () => false */) {
    scene.add.nineslice(960, 1152, 'box_orange_square', undefined, 760, 170, 32, 32, 32, 32).setDepth(30);

    const birds = ['sparrow', 'woodpecker', 'eagle', 'peacock'];
    const startX = 637;
    const boxY = 1152;
    const boxSize = 136;
    const headSize = 98;

    birds.forEach((bird, index) => {
      const boxX = startX + index * 170 + boxSize / 2;
      const stats = BIRD_STATS[bird];

      const tooltip = scene.add.container(boxX, boxY - 230).setDepth(35).setVisible(false);
      tooltip.add(scene.add.nineslice(0, 0, 'box_square', undefined, 300, 220, 32, 32, 32, 32));
      tooltip.add(scene.add.text(0, -82, bird.toUpperCase(), {
        fontFamily: 'Concert One', fontSize: '32px', color: '#070505',
      }).setOrigin(0.5));

      const rows = [
        { label: 'DAMAGE',    value: String(stats.damage),  color: '#f87171' },
        { label: 'RANGE',     value: String(stats.range),   color: '#60a5fa' },
        { label: 'FIRE RATE', value: stats.fireRate,        color: '#34d399' },
        { label: 'ATTACK',    value: stats.attack,          color: '#e9d5ff' },
        { label: 'COST',      value: String(stats.cost),    color: '#fbbf24' },
      ];
      rows.forEach((row, i) => {
        const rowY = -42 + i * 28;
        tooltip.add(scene.add.text(-120, rowY, row.label, {
          fontFamily: 'Concert One', fontSize: '20px', color: '#94a3b8',
        }).setOrigin(0, 0.5));
        tooltip.add(scene.add.text(120, rowY, row.value, {
          fontFamily: 'Concert One', fontSize: '20px', color: row.color,
        }).setOrigin(1, 0.5));
      });

      const box = scene.add.nineslice(boxX, boxY, 'box_square', undefined, boxSize, boxSize, 32, 32, 32, 32)
        .setInteractive({ useHandCursor: true, draggable: true })
        .setData('birdType', bird).setData('tooltip', tooltip).setDepth(30);

      const head  = scene.add.image(boxX, boxY - 14, `head_${bird}`).setDisplaySize(headSize, headSize).setDepth(31);
      const label = scene.add.text(boxX, boxY + 44, bird.toUpperCase(), {
        fontFamily: 'Concert One', fontSize: '19px', color: '#94a3b8',
      }).setOrigin(0.5).setDepth(31);

      box.on('pointerover', () => {
        if (isDragging()) return;
        box.setSize(boxSize + 12, boxSize + 12);
        head.setTexture(`head_${bird}`);
        head.setDisplaySize(headSize + 8, headSize + 8).setY(boxY - 18);
        label.setColor('#ffffff').setY(boxY + 50);
        tooltip.setVisible(true);
      });
      box.on('pointerout', () => {
        box.setSize(boxSize, boxSize);
        head.setTexture(`head_${bird}`);
        head.setDisplaySize(headSize, headSize).setY(boxY - 14);
        label.setColor('#94a3b8').setY(boxY + 44);
        tooltip.setVisible(false);
      });
    });
  }
}
