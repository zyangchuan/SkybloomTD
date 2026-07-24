import Phaser from 'phaser';

export default class Tower extends Phaser.GameObjects.Sprite {
  public id: string;
  public birdType: string;
  public gridX: number;
  public gridY: number;
  public range: number = 0;
  public lifespan: number = 0;
  public spread: number = 0;

  constructor(scene: Phaser.Scene, x: number, y: number, id: string, birdType: string, gridX: number, gridY: number) {
    super(scene, x, y, `tower_${birdType}`);
    this.id = id;
    this.birdType = birdType;
    this.gridX = gridX;
    this.gridY = gridY;

    this.setOrigin(0.5, 0.5);

    scene.add.existing(this);
    this.setInteractive({ useHandCursor: true, draggable: true });
    scene.input.setDraggable(this);
  }

  public rotateTower(angle: number) {
    this.setRotation(angle);
  }

  public update() {
    const gameScene = this.scene as any;
    const enemies = gameScene?.entities?.activeEnemiesList ?? [];
    if (enemies.length === 0) {
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
      return;
    }

    let bestEnemy: any = null;

    for (const enemy of enemies) {
      const dx = enemy.x - this.gridX;
      const dy = enemy.y - this.gridY;
      const distance = Math.sqrt(dx * dx + dy * dy);

      if (distance <= this.range) {
        if (!bestEnemy || enemy.pathIndex > bestEnemy.pathIndex) {
          bestEnemy = enemy;
        }
      }
    }

    if (bestEnemy) {
      const targetAngle = Math.atan2(bestEnemy.y - this.gridY, bestEnemy.x - this.gridX) + Math.PI;
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, targetAngle, 0.08);
    } else {
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
    }
  }
}
