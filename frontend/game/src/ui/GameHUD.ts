import Phaser from 'phaser';

export class GameHUD {
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;

  constructor(scene: Phaser.Scene, onPause: () => void, onAlmanac: () => void) {
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

    const almanacButton = scene.add.image(scene.scale.width - 300, 100, 'almanac').setDepth(90).setInteractive({ useHandCursor: true }).setRotation(0.3).setScale(0.15);
    almanacButton.on('pointerover', () => almanacButton.setScale(0.17));
    almanacButton.on('pointerout', () => almanacButton.setScale(0.15));
    almanacButton.on('pointerdown', onAlmanac);

    const pauseBtnBg = scene.add
      .nineslice(scene.scale.width - 90, 80, 'box_orange_square', undefined, 120, 110, 32, 32, 32, 32)
      .setDepth(30)
      .setScale(1.0)
      .setInteractive({ useHandCursor: true });
    const icon_pause = scene.add.image(scene.scale.width - 90, 80, 'icon_pause').setScale(0.5).setDepth(31);
   
    pauseBtnBg.on('pointerover', () => { icon_pause.setScale(0.525); pauseBtnBg.setScale(1.1) });
    pauseBtnBg.on('pointerout', () => { icon_pause.setScale(0.5); pauseBtnBg.setScale(1.0) });
    pauseBtnBg.on('pointerdown', onPause);
  }

  update(state: { health?: number, essence?: number, wave?: number }) {
    if (state.health  !== undefined) this.healthText.setText(String(state.health));
    if (state.essence !== undefined) this.essenceText.setText(String(state.essence));
    if (state.wave    !== undefined) this.waveText.setText(`WAVE ${state.wave}`);
  }
}
