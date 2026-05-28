import Phaser from 'phaser';

export default class Tower extends Phaser.GameObjects.Sprite {
  public id: string;
  public birdType: string;
  public gridX: number;
  public gridY: number;

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
   * Slowly rotate in idle to feel highly interactive and dynamic (disabled by request)
   */
  public update() {
    // Rotation remains stable at 0 by default, tracking to be implemented later.
  }
}
