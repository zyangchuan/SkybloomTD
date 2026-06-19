import Phaser from 'phaser';

export class GameHUD {
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;

  constructor(scene: Phaser.Scene, onPause: () => void) {
    scene.add.nineslice(360, 69, 'box_orange_square', undefined, 700, 90, 32, 32, 32, 32).setDepth(30);

    scene.add.image(62, 69, 'icon_heart').setDisplaySize(50, 50).setDepth(30);
    this.healthText = scene.add.text(124, 69, '100', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px', color: '#7c0000ff',
    }).setOrigin(0, 0.5).setDepth(30);

    scene.add.image(274, 69, 'icon_essence').setDisplaySize(50, 50).setDepth(30);
    this.essenceText = scene.add.text(336, 69, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px', color: '#a35700ff',
    }).setOrigin(0, 0.5).setDepth(30);

    this.waveText = scene.add.text(484, 69, 'WAVE 0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px', color: '#451a03',
    }).setOrigin(0, 0.5).setDepth(30);

    const pauseBtnBg = scene.add
      .nineslice(1840, 69, 'box_orange_square', undefined, 100, 90, 32, 32, 32, 32)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });
    scene.add.image(1840, 69, 'icon_pause').setDisplaySize(50, 50).setDepth(31);
    pauseBtnBg.on('pointerdown', onPause);
  }

  update(state: { health?: number, essence?: number, wave?: number }) {
    if (state.health  !== undefined) this.healthText.setText(String(state.health));
    if (state.essence !== undefined) this.essenceText.setText(String(state.essence));
    if (state.wave    !== undefined) this.waveText.setText(`WAVE ${state.wave}`);
  }
}
