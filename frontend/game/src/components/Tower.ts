import Phaser from 'phaser';

export default class Tower extends Phaser.GameObjects.Sprite {
  public id: string;
  public birdType: string;
  public gridX: number;
  public gridY: number;
  public range: number = 0;

  constructor(scene: Phaser.Scene, x: number, y: number, id: string, birdType: string, gridX: number, gridY: number) {
    super(scene, x, y, `tower_${birdType}`);
    this.id = id;
    this.birdType = birdType;
    this.gridX = gridX;
    this.gridY = gridY;

    // Center origin for flawless rotation dynamics
    this.setOrigin(0.5, 0.5);

    scene.add.existing(this);
  }

  /**
   * Directly sets the tower rotation angle (in radians)
   */
  public rotateTower(angle: number) {
    this.setRotation(angle);
  }

  /**
   * Targets the enemy closest to the end (furthest along path) within its range
   * and smoothly interpolates rotation towards it.
   */
  public update() {
    const gameScene = this.scene as any;
    const enemies = gameScene?.entities?.activeEnemiesList ?? [];
    if (enemies.length === 0) {
      // Return slowly to default rotation of 0 when no enemies exist
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
      return;
    }

    let bestEnemy: any = null;

    // Standard high-performance target selection mirroring the Go server
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
      // Calculate rotation target angle directly using grid coordinates
      const targetAngle = Math.atan2(bestEnemy.y - this.gridY, bestEnemy.x - this.gridX) + Math.PI;

      // Interpolate smoothly towards the target angle
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, targetAngle, 0.08);
    } else {
      // Return slowly to default rotation of 0 when no target is in range
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
    }
  }
}
