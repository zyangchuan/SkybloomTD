import Phaser from 'phaser';
import { BIRD_STATS } from '../data/birds';

export class BirdTray {
  constructor(scene: Phaser.Scene, isDragging: () => boolean) {
    const width = scene.scale.width;
    const height = scene.scale.height;
    const centerX = width / 2;
    const boxY = height - 145;
    scene.add.nineslice(centerX, boxY, 'box_orange_square', undefined, 900, 200, 32, 32, 32, 32).setDepth(30);

    const birds = ['sparrow', 'woodpecker', 'eagle', 'peacock'];
    const startX = centerX - 380;
    const boxSize = 160;
    const headSize = 115;

    birds.forEach((bird, index) => {
      const boxX = startX + index * 200 + boxSize / 2;
      const stats = BIRD_STATS[bird];

      const tooltip = scene.add.container(boxX, boxY - 260).setDepth(35).setVisible(false);
      tooltip.add(scene.add.nineslice(0, 0, 'box_square', undefined, 350, 250, 32, 32, 32, 32));
      tooltip.add(scene.add.text(0, -95, bird.toUpperCase(), {
        fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '38px', color: stats.color,
      }).setOrigin(0.5));

      const rows = [
        { label: 'DAMAGE',    value: String(stats.damage),  color: '#f87171' },
        { label: 'RANGE',     value: String(stats.range),   color: '#60a5fa' },
        { label: 'FIRE RATE', value: stats.fireRate,        color: '#34d399' },
        { label: 'ATTACK',    value: stats.attack,          color: '#e9d5ff' },
        { label: 'COST',      value: String(stats.cost),    color: '#fbbf24' },
      ];
      rows.forEach((row, i) => {
        const rowY = -50 + i * 32;
        tooltip.add(scene.add.text(-140, rowY, row.label, {
          fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '24px', color: '#94a3b8',
        }).setOrigin(0, 0.5));
        tooltip.add(scene.add.text(140, rowY, row.value, {
          fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '24px', color: row.color,
        }).setOrigin(1, 0.5));
      });

      const box = scene.add.nineslice(boxX, boxY, 'box_square', undefined, boxSize, boxSize, 32, 32, 32, 32)
        .setInteractive({ useHandCursor: true, draggable: true })
        .setData('birdType', bird).setData('tooltip', tooltip).setDepth(30);

      const head  = scene.add.image(boxX, boxY - 18, `head_${bird}`).setDisplaySize(headSize, headSize).setDepth(31);
      const label = scene.add.text(boxX, boxY + 54, bird.toUpperCase(), {
        fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '22px', color: '#94a3b8',
      }).setOrigin(0.5).setDepth(31);

      box.on('pointerover', () => {
        if (isDragging()) return;
        box.setSize(boxSize + 15, boxSize + 15);
        head.setDisplaySize(headSize + 10, headSize + 10).setY(boxY - 22);
        label.setColor('#ffffff').setY(boxY + 60);
        tooltip.setVisible(true);
      });
      box.on('pointerout', () => {
        box.setSize(boxSize, boxSize);
        head.setDisplaySize(headSize, headSize).setY(boxY - 18);
        label.setColor('#94a3b8').setY(boxY + 54);
        tooltip.setVisible(false);
      });
    });
  }
}
