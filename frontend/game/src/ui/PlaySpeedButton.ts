import Phaser from 'phaser';

export class PlaySpeedButton {
  private buttonBg!: Phaser.GameObjects.NineSlice;
  private pointer1!: Phaser.GameObjects.Image;
  private pointer2!: Phaser.GameObjects.Image;
  private isFast = false;

  constructor(
    private scene: Phaser.Scene,
    private send: (type: string, data?: object) => void
  ) {
    this.create();
  }

  private create() {
    const { width } = this.scene.scale;
    const x = width - 230; // Placed 140px to the left of Pause Button (width - 90)
    const y = 80;          // Aligned with the top HUD panel Y level

    // Create the green square icon button background using NineSlice
    // Dimensions are 120x110, exactly matching the Pause Button size
    this.buttonBg = this.scene.add.nineslice(x, y, 'btn_green_square', undefined, 120, 110, 16, 16, 16, 16).setDepth(30);
    this.buttonBg.setInteractive({ useHandCursor: true });

    // Create the two pointers
    // Rotation is 90 degrees anti-clockwise (which is -90 degrees in angle)
    this.pointer1 = this.scene.add.image(x, y, 'scroll_pointer')
      .setDepth(31)
      .setAngle(-90);

    this.pointer2 = this.scene.add.image(x, y, 'scroll_pointer')
      .setDepth(31)
      .setAngle(-90)
      .setVisible(false);

    // Hover effect functions
    const onOver = () => {
      this.buttonBg.setScale(1.08);
      const currentSize = this.isFast ? 44 : 60;
      this.pointer1.setDisplaySize(currentSize * 1.08, currentSize * 1.08);
      this.pointer2.setDisplaySize(currentSize * 1.08, currentSize * 1.08);
    };

    const onOut = () => {
      this.buttonBg.setScale(1.0);
      const currentSize = this.isFast ? 44 : 60;
      this.pointer1.setDisplaySize(currentSize, currentSize);
      this.pointer2.setDisplaySize(currentSize, currentSize);
    };

    const onClick = () => {
      this.isFast = !this.isFast;
      if (this.isFast) {
        this.send('game.speed.fast');
        this.buttonBg.setTint(0xfffacd); // light highlight
      } else {
        this.send('game.speed.normal');
        this.buttonBg.clearTint();
      }
      this.updateState();
    };

    // Bind listeners
    this.buttonBg.on('pointerover', onOver);
    this.buttonBg.on('pointerout', onOut);
    this.buttonBg.on('pointerdown', onClick);

    this.updateState();
  }

  private updateState() {
    const x = this.buttonBg.x;
    if (this.isFast) {
      // 2 pointers side-by-side, sized 44px, offset by 14px (close overlap for >> cohesion)
      this.pointer1.setDisplaySize(44, 44).setX(x - 14).setVisible(true);
      this.pointer2.setDisplaySize(44, 44).setX(x + 14).setVisible(true);
    } else {
      // 1 pointer centered, sized 60px
      this.pointer1.setDisplaySize(60, 60).setX(x).setVisible(true);
      this.pointer2.setVisible(false);
    }
  }

  destroy() {
    this.buttonBg.destroy();
    this.pointer1.destroy();
    this.pointer2.destroy();
  }
}
