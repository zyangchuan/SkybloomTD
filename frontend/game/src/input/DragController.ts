import Phaser from 'phaser';
import Tower from '../components/Tower';
import { BIRD_STATS } from '../data/birds';

interface GridParams {
  tileSize: number;
  offsetX: number;
  offsetY: number;
  gridWidth: number;
  gridHeight: number;
}

export class DragController {
  private activeDragSprite: Phaser.GameObjects.Sprite | null = null;
  private activeDragBirdType: string | null = null;
  private activeDragTooltip: Phaser.GameObjects.Container | null = null;
  private gridHighlightGraphics: Phaser.GameObjects.Graphics;
  private closestCellHighlight: Phaser.GameObjects.Graphics;
  private dragRangeGraphics: Phaser.GameObjects.Graphics | null = null;
  private pulseTween: Phaser.Tweens.Tween | null = null;
  private dragCancelLeftBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelLeftText: Phaser.GameObjects.Text | null = null;
  private dragCancelRightBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelRightText: Phaser.GameObjects.Text | null = null;

  constructor(
    private scene: Phaser.Scene,
    private onPlace: (birdType: string, x: number, y: number) => void,
    private grid: GridParams,
    private enemyPath: any[],
    private obstacles: any[],
    private towers: Map<string, Tower>,
  ) {
    this.gridHighlightGraphics = scene.add.graphics().setDepth(5);
    this.closestCellHighlight  = scene.add.graphics().setDepth(6);
  }

  isDragging() { return this.activeDragBirdType !== null; }

  setup() {
    this.scene.input.on('dragstart', this.onDragStart, this);
    this.scene.input.on('drag',      this.onDrag,      this);
    this.scene.input.on('dragend',   this.onDragEnd,   this);
  }

  // ─── Event handlers ─────────────────────────────────────────────────────────

  private onDragStart(pointer: Phaser.Input.Pointer, gameObject: Phaser.GameObjects.GameObject) {
    const birdType = gameObject.getData('birdType');
    if (!birdType) return;

    this.activeDragBirdType = birdType;

    const tooltip = gameObject.getData('tooltip') as Phaser.GameObjects.Container;
    if (tooltip) {
      tooltip.setVisible(true).setDepth(45);
      this.activeDragTooltip = tooltip;
    }

    this.dragRangeGraphics = this.scene.add.graphics().setDepth(39);
    this.activeDragSprite = this.scene.add.sprite(pointer.x, pointer.y, `tower_${birdType}`)
      .setAlpha(0.8).setDepth(40);
    this.activeDragSprite.setScale(this.grid.tileSize / this.activeDragSprite.width * 1.5);

    this.spawnCancelZones();
    this.drawGrassHighlights();
    this.gridHighlightGraphics.setAlpha(0.2);
    this.pulseTween = this.scene.tweens.add({
      targets: this.gridHighlightGraphics,
      alpha: { from: 0.2, to: 0.7 },
      duration: 700, yoyo: true, repeat: -1, ease: 'Sine.easeInOut',
    });
  }

  private onDrag(pointer: Phaser.Input.Pointer, _gameObject: Phaser.GameObjects.GameObject) {
    if (!this.activeDragSprite) return;

    this.activeDragSprite.setPosition(pointer.x, pointer.y);
    this.updateRangeCircle(pointer);
    this.updateCancelZones(pointer);
    this.updateCellHighlight(pointer);
  }

  private onDragEnd(pointer: Phaser.Input.Pointer) {
    this.dragRangeGraphics?.destroy();
    this.dragRangeGraphics = null;
    if (!this.activeDragSprite) return;

    const { gridX, gridY } = this.toGrid(pointer.x, pointer.y);
    const inBirdsBar = pointer.y > 1050;

    if (!inBirdsBar && this.isValidGrassTile(gridX, gridY) && this.activeDragBirdType) {
      this.onPlace(this.activeDragBirdType, gridX, gridY);
    }

    this.activeDragTooltip?.setVisible(false).setDepth(35);
    this.activeDragTooltip = null;
    this.activeDragSprite.destroy();
    this.activeDragSprite = null;
    this.activeDragBirdType = null;

    this.destroyCancelZones();
    this.pulseTween?.stop();
    this.pulseTween = null;
    this.gridHighlightGraphics.clear().setAlpha(1.0);
    this.closestCellHighlight.clear();
  }

  // ─── Drag helpers ────────────────────────────────────────────────────────────

  private updateRangeCircle(pointer: Phaser.Input.Pointer) {
    if (!this.dragRangeGraphics || !this.activeDragBirdType) return;
    this.dragRangeGraphics.clear();
    const stats = BIRD_STATS[this.activeDragBirdType];
    if (!stats) return;
    const radius = stats.range * this.grid.tileSize;
    this.dragRangeGraphics.fillStyle(0x93c5fd, 0.3);
    this.dragRangeGraphics.fillCircle(pointer.x, pointer.y, radius);
    this.dragRangeGraphics.lineStyle(3, 0x2563eb, 0.8);
    this.dragRangeGraphics.strokeCircle(pointer.x, pointer.y, radius);
  }

  private updateCancelZones(pointer: Phaser.Input.Pointer) {
    const hoverLeft  = pointer.y > 1050 && pointer.x < 580;
    const hoverRight = pointer.y > 1050 && pointer.x > 1340;

    if (this.dragCancelLeftBox && this.dragCancelLeftText) {
      this.dragCancelLeftBox.setAlpha(hoverLeft ? 0.9 : 0.75);
      this.dragCancelLeftText.setScale(hoverLeft ? 1.15 : 1.0).setColor(hoverLeft ? '#fecaca' : '#ffffff');
    }
    if (this.dragCancelRightBox && this.dragCancelRightText) {
      this.dragCancelRightBox.setAlpha(hoverRight ? 0.9 : 0.75);
      this.dragCancelRightText.setScale(hoverRight ? 1.15 : 1.0).setColor(hoverRight ? '#fecaca' : '#ffffff');
    }
  }

  private updateCellHighlight(pointer: Phaser.Input.Pointer) {
    const { gridX, gridY } = this.toGrid(pointer.x, pointer.y);
    const inBirdsBar = pointer.y > 1050;
    const { tileSize, offsetX, offsetY, gridWidth, gridHeight } = this.grid;

    this.closestCellHighlight.clear();

    if (!inBirdsBar && this.isValidGrassTile(gridX, gridY)) {
      const posX = offsetX + gridX * tileSize;
      const posY = offsetY + gridY * tileSize;
      this.closestCellHighlight.fillStyle(0x34d399, 0.45).lineStyle(3, 0x10b981, 1);
      this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
      this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
    } else if (!inBirdsBar && gridX >= 0 && gridX < gridWidth && gridY >= 0 && gridY < gridHeight) {
      const posX = offsetX + gridX * tileSize;
      const posY = offsetY + gridY * tileSize;
      this.closestCellHighlight.fillStyle(0xf87171, 0.35).lineStyle(3, 0xef4444, 1);
      this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
      this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
    }
  }

  private spawnCancelZones() {
    this.dragCancelLeftBox = this.scene.add.graphics().setDepth(30);
    this.dragCancelLeftBox.fillStyle(0xef4444, 0.6);
    this.dragCancelLeftBox.fillRoundedRect(280 - 270, 1152 - 85, 540, 170, 24);
    this.dragCancelLeftBox.setAlpha(0.75);
    this.dragCancelLeftText = this.scene.add.text(280, 1152, 'RELEASE TO CANCEL', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '26px', color: '#ffffff',
    }).setOrigin(0.5).setDepth(31);

    this.dragCancelRightBox = this.scene.add.graphics().setDepth(30);
    this.dragCancelRightBox.fillStyle(0xef4444, 0.6);
    this.dragCancelRightBox.fillRoundedRect(1640 - 270, 1152 - 85, 540, 170, 24);
    this.dragCancelRightBox.setAlpha(0.75);
    this.dragCancelRightText = this.scene.add.text(1640, 1152, 'RELEASE TO CANCEL', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '26px', color: '#ffffff',
    }).setOrigin(0.5).setDepth(31);
  }

  private destroyCancelZones() {
    this.dragCancelLeftBox?.destroy();  this.dragCancelLeftBox = null;
    this.dragCancelLeftText?.destroy(); this.dragCancelLeftText = null;
    this.dragCancelRightBox?.destroy(); this.dragCancelRightBox = null;
    this.dragCancelRightText?.destroy(); this.dragCancelRightText = null;
  }

  private drawGrassHighlights() {
    const { tileSize, offsetX, offsetY, gridWidth, gridHeight } = this.grid;
    this.gridHighlightGraphics.clear().fillStyle(0xffffff, 0.3).lineStyle(2, 0xffffff, 1.0);
    for (let y = 0; y < gridHeight; y++) {
      for (let x = 0; x < gridWidth; x++) {
        if (this.isValidGrassTile(x, y)) {
          const posX = offsetX + x * tileSize;
          const posY = offsetY + y * tileSize;
          this.gridHighlightGraphics.fillRect(posX, posY, tileSize, tileSize);
          this.gridHighlightGraphics.strokeRect(posX, posY, tileSize, tileSize);
        }
      }
    }
  }

  isValidGrassTile(x: number, y: number): boolean {
    const { gridWidth, gridHeight } = this.grid;
    if (x < 0 || x >= gridWidth || y < 0 || y >= gridHeight) return false;
    if (this.enemyPath.some((t: any) => t.x === x && t.y === y)) return false;
    if (this.obstacles.some((o: any) => o.x === x && o.y === y)) return false;
    for (const tower of this.towers.values()) {
      if (tower.gridX === x && tower.gridY === y) return false;
    }
    return true;
  }

  private toGrid(px: number, py: number) {
    return {
      gridX: Math.floor((px - this.grid.offsetX) / this.grid.tileSize),
      gridY: Math.floor((py - this.grid.offsetY) / this.grid.tileSize),
    };
  }
}
