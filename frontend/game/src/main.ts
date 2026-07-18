import Phaser from 'phaser';
import BootScene from './scenes/BootScene';
import GameScene from './scenes/GameScene';

const config: Phaser.Types.Core.GameConfig & { resolution?: number } = {
  type: Phaser.AUTO,
  width: 2400,
  height: 1600,
  resolution: Math.min(window.devicePixelRatio || 1, 2),
  parent: 'game-container',
  backgroundColor: '#0a0e17',
  scale: {
    mode: Phaser.Scale.FIT,
    autoCenter: Phaser.Scale.CENTER_BOTH,
  },
  scene: [BootScene, GameScene],
};

const game = new Phaser.Game(config);

window.addEventListener('resize', () => {
  if (game && game.scale) {
    game.scale.refresh();
  }
});

window.addEventListener('orientationchange', () => {
  setTimeout(() => {
    if (game && game.scale) {
      game.scale.refresh();
    }
  }, 200);
});
