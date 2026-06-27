import Phaser from 'phaser';

export class GameHUD {
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;

  constructor(scene: Phaser.Scene, onPause: () => void) {
    scene.add.nineslice(420, 80, 'box_orange_square', undefined, 820, 110, 32, 32, 32, 32).setDepth(30);

    scene.add.image(75, 80, 'icon_heart').setDisplaySize(60, 60).setDepth(30);
    this.healthText = scene.add.text(125, 80, '100', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '64px', color: '#7c0000ff',
    }).setOrigin(0, 0.5).setDepth(30);

    scene.add.image(310, 80, 'icon_essence').setDisplaySize(78, 78).setDepth(30);
    this.essenceText = scene.add.text(370, 80, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '74px', color: '#966600ff',
    }).setOrigin(0, 0.5).setDepth(30);

    this.waveText = scene.add.text(525, 80, 'WAVE 0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '64px', color: '#451a03',
    }).setOrigin(0, 0.5).setDepth(30);

    const pauseBtnBg = scene.add
      .nineslice(scene.scale.width - 90, 80, 'box_orange_square', undefined, 120, 110, 32, 32, 32, 32)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });
    scene.add.image(scene.scale.width - 90, 80, 'icon_pause').setDisplaySize(60, 60).setDepth(31);
    pauseBtnBg.on('pointerdown', onPause);
  }

  update(state: { health?: number, essence?: number, wave?: number }) {
    if (state.health  !== undefined) this.healthText.setText(String(state.health));
    if (state.essence !== undefined) this.essenceText.setText(String(state.essence));
    if (state.wave    !== undefined) this.waveText.setText(`WAVE ${state.wave}`);
  }
}
