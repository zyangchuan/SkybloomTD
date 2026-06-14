import Phaser from 'phaser';
import { speed, normalSpeed } from '../SpeedGame';

export class GameHUD {
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;
  private speedText: Phaser.GameObjects.Text;

  constructor(scene: Phaser.Scene, onPause: () => void, norSpeed : boolean, onSpeedChange: (isDouble: boolean) => void) {
    scene.add.nineslice(360, 69, 'box_orange_square', undefined, 700, 90, 32, 32, 32, 32).setDepth(30);

    scene.add.image(62, 69, 'icon_heart').setDisplaySize(50, 50).setDepth(30);
    this.healthText = scene.add.text(124, 69, '100', {
      fontFamily: "Concert One",
      fontSize: '54px', color: '#7c0000ff',
    }).setOrigin(0, 0.5).setDepth(30);

    scene.add.image(274, 69, 'icon_essence').setDisplaySize(50, 50).setDepth(30);
    this.essenceText = scene.add.text(336, 69, '0', {
      fontFamily: "Concert One",
      fontSize: '54px', color: '#a35700ff',
    }).setOrigin(0, 0.5).setDepth(30);

    this.waveText = scene.add.text(484, 69, 'WAVE 0', {
      fontFamily: "Concert One",
      fontSize: '54px', color: '#451a03',
    }).setOrigin(0, 0.5).setDepth(30);

    const pauseBtnBg = scene.add
      .nineslice(1840, 69, 'box_orange_square', undefined, 100, 90, 32, 32, 32, 32)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });
   
    const icon_pause = scene.add.image(1840, 69, 'icon_pause').setDisplaySize(50, 50).setDepth(31);

    pauseBtnBg.on('pointerover', () => { pauseBtnBg.setScale(1.1); icon_pause.setDisplaySize(60, 60); });
    pauseBtnBg.on('pointerout', () => { pauseBtnBg.setScale(1.0); icon_pause.setDisplaySize(50, 50); })
    pauseBtnBg.on('pointerdown', onPause);

    const speedBtnBg = scene.add
      .nineslice(1700, 69, 'box_orange_square', undefined, 100, 90, 32, 32, 32, 32)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });

    this.speedText = scene.add
      .text(1700, 69, 'X1', { fontSize: '50px', fontFamily: 'Concert One', color: '#000000'}).setOrigin(0.5).setDepth(130);

    speedBtnBg.on('pointerover', () => { speedBtnBg.setScale(1.1); this.speedText.setScale(1.1).setColor('#f3f1f3'); });
    speedBtnBg.on('pointerout', () => { speedBtnBg.setScale(1.0); this.speedText.setScale(1.0).setColor('#000000'); })
    speedBtnBg.on('pointerdown', () => {
      norSpeed = !norSpeed;

     if (norSpeed) {
      this.speedText.setText('X1');
      normalSpeed(scene);
      onSpeedChange(false);
     } else {
      this.speedText.setText('X2');
      speed(scene);
      onSpeedChange(true);
     }

    } );
  }

  update(state: { health?: number, essence?: number, wave?: number }) {
    if (state.health  !== undefined) this.healthText.setText(String(state.health));
    if (state.essence !== undefined) this.essenceText.setText(String(state.essence));
    if (state.wave    !== undefined) this.waveText.setText(`WAVE ${state.wave}`);
  }
}
