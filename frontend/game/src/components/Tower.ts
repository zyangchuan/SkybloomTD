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
    if (!gameScene || !gameScene.activeSmogsList || gameScene.activeSmogsList.length === 0) {
      // Return slowly to default rotation of 0 when no smogs exist
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
      return;
    }

    let bestSmog: any = null;

    // Standard high-performance target selection mirroring the Go server
    for (const smog of gameScene.activeSmogsList) {
      const dx = smog.x - this.gridX;
      const dy = smog.y - this.gridY;
      const distance = Math.sqrt(dx * dx + dy * dy);

      if (distance <= this.range) {
        if (!bestSmog || smog.pathIndex > bestSmog.pathIndex) {
          bestSmog = smog;
        }
      }
    }

    if (bestSmog) {
      // Calculate rotation target angle directly using grid coordinates
      const targetAngle = Math.atan2(bestSmog.y - this.gridY, bestSmog.x - this.gridX) + Math.PI;

      // Interpolate smoothly towards the target angle
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, targetAngle, 0.08);
    } else {
      // Return slowly to default rotation of 0 when no target is in range
      this.rotation = Phaser.Math.Angle.RotateTo(this.rotation, 0, 0.04);
    }
  }
}
