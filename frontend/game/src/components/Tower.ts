import Phaser from 'phaser';

export default class Tower extends Phaser.GameObjects.Sprite {
  public id: string;
  public birdType: string;
  public gridX: number;
  public gridY: number;
  public range: number = 0;
  public evolveLevel: number = 0;
  public tileSize: number = 0;

  constructor(scene: Phaser.Scene, x: number, y: number, id: string, birdType: string, gridX: number, gridY: number) {
    super(scene, x, y, `tower_${birdType}`);
    this.id = id;
    this.birdType = birdType;
    this.gridX = gridX;
    this.gridY = gridY;
    this.setOrigin(0.5, 0.5);
    scene.add.existing(this);
  }

  // Called when server confirms an evolution (tower type changes to evolve_X)
  evolve() {
    if (this.evolveLevel >= 1) return;
    this.evolveLevel = 1;
    const base = this.birdType;
    this.birdType = `evolve_${base}`;
    this.setTexture(`tower_evolve_${base}`);

    if (this.tileSize > 0) {
      const scale = this.tileSize / this.width * 1.9;
      this.setScale(scale);
      this.scene.tweens.add({
        targets: this, scaleX: scale * 1.5, scaleY: scale * 1.5,
        duration: 200, yoyo: true
      });
    }
  }

  rotateTower(angle: number) {
    this.setRotation(angle);
  }

  update(activeSmogsList: Array<{ id: string; x: number; y: number; pathIndex: number }>) {
    if (!activeSmogsList.length) {
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
      return;
    }

    let bestSmog: any = null;
    for (const smog of activeSmogsList) {
      const dx = smog.x - this.gridX;
      const dy = smog.y - this.gridY;
      if (Math.sqrt(dx * dx + dy * dy) <= this.range) {
        if (!bestSmog || smog.pathIndex > bestSmog.pathIndex) bestSmog = smog;
      }
    }

    if (bestSmog) {
      this.rotation = Phaser.Math.Angle.RotateTo(
        this.rotation,
        Math.atan2(bestSmog.y - this.gridY, bestSmog.x - this.gridX) + Math.PI,
        0.08,
      );
    } else {
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
    }
  }
}
