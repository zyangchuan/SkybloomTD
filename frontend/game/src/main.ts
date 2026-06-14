import Phaser from 'phaser';
import BootScene from './scenes/BootScene';
import GameScene from './scenes/GameScene';

const config: Phaser.Types.Core.GameConfig = {
  type: Phaser.AUTO,
  width: 1920,
  height: 1280,
  parent: 'game-container',
  backgroundColor: '#0a0e17',
  scale: {
    mode: Phaser.Scale.FIT,
    autoCenter: Phaser.Scale.CENTER_BOTH,
  },
  scene: [BootScene, GameScene],
};

const game = new Phaser.Game(config);

// Bind simple dynamic resize listener to refresh scale perfectly when screen metrics change on mobile
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
  }, 200); // 200ms delay to let mobile viewport calculations settle before refresh
});
