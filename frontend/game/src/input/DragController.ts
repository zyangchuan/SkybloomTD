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

const MERGE_RECIPES: Record<string, Record<string, string>> = {
  sparrow: { eagle: 'falcon' },
  eagle: { sparrow: 'falcon', peacock: 'phoenix', kingfisher: 'sun_god' },
  woodpecker: { peacock: 'kingfisher' },
  peacock: { woodpecker: 'kingfisher', eagle: 'phoenix' },
  kingfisher: { eagle: 'sun_god' },
};

function getMergeResult(typeA: string, typeB: string): string | null {
  if (MERGE_RECIPES[typeA] && MERGE_RECIPES[typeA][typeB]) {
    return MERGE_RECIPES[typeA][typeB];
  }
  return null;
}

export class DragController {
  private activeDragSprite: Phaser.GameObjects.Sprite | null = null;
  private activeDragBirdType: string | null = null;
  private activeDragTooltip: Phaser.GameObjects.Container | null = null;
  private mergeTooltip: Phaser.GameObjects.Container | null = null;
  private gridHighlightGraphics: Phaser.GameObjects.Graphics;
  private closestCellHighlight: Phaser.GameObjects.Graphics;
  private dragRangeGraphics: Phaser.GameObjects.Graphics | null = null;
  private pulseTween: Phaser.Tweens.Tween | null = null;
  private dragCancelLeftBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelLeftText: Phaser.GameObjects.Text | null = null;
  private dragCancelRightBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelRightText: Phaser.GameObjects.Text | null = null;

  private activeDragTower: Tower | null = null;
  private activeDragTowerSourceId: string | null = null;

  constructor(
    private scene: Phaser.Scene,
    private onPlace: (birdType: string, x: number, y: number) => void,
    private onMergeExisting: (sourceId: string, targetId: string) => void,
    private onMergeBought: (sourceBirdType: string, targetId: string) => void,
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
    let birdType: string | null = null;
    const isTower = gameObject instanceof Tower || ((gameObject as any).birdType !== undefined && (gameObject as any).id !== undefined);

    if (isTower) {
      const tower = gameObject as Tower;
      birdType = tower.birdType;
      this.activeDragTower = tower;
      this.activeDragTowerSourceId = tower.id;
      tower.setAlpha(0.4);
    } else {
      birdType = gameObject.getData('birdType');
      this.activeDragTower = null;
      this.activeDragTowerSourceId = null;
    }

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
    let dragScaleMultiplier = 1.5;
    if (birdType === 'sun_god') {
      dragScaleMultiplier = 2.5;
    } else if (birdType === 'phoenix') {
      dragScaleMultiplier = 2.2;
    } else if (birdType === 'kingfisher' || birdType === 'falcon') {
      dragScaleMultiplier = 2.0;
    }
    this.activeDragSprite.setScale(this.grid.tileSize / this.activeDragSprite.width * dragScaleMultiplier);

    this.spawnCancelZones();
    if (this.activeDragTowerSourceId) {
      this.drawMergeHighlights();
    } else {
      this.drawPlacementAndMergeHighlights();
    }
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
    const inBirdsBar = pointer.y > (this.scene.scale.height - 245);

    if (this.activeDragTowerSourceId) {
      if (this.activeDragTower) {
        this.activeDragTower.setAlpha(1.0);
      }

      let targetTower: Tower | null = null;
      for (const t of this.towers.values()) {
        if (t.gridX === gridX && t.gridY === gridY && t.id !== this.activeDragTowerSourceId) {
          targetTower = t;
          break;
        }
      }

      if (targetTower && this.activeDragBirdType) {
        const resultType = getMergeResult(this.activeDragBirdType, targetTower.birdType);
        if (resultType) {
          this.onMergeExisting(this.activeDragTowerSourceId, targetTower.id);
        }
      }

      this.activeDragTower = null;
      this.activeDragTowerSourceId = null;
    } else {
      if (!inBirdsBar && this.activeDragBirdType) {
        const targetTower = this.findTowerAt(gridX, gridY);
        if (targetTower && getMergeResult(this.activeDragBirdType, targetTower.birdType)) {
          this.onMergeBought(this.activeDragBirdType, targetTower.id);
        } else if (this.isValidGrassTile(gridX, gridY)) {
          this.onPlace(this.activeDragBirdType, gridX, gridY);
        }
      }
    }

    this.activeDragTooltip?.setVisible(false).setDepth(35);
    this.activeDragTooltip = null;
    this.hideMergeTooltip();
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
    const inBirdsBar = pointer.y > (this.scene.scale.height - 245);
    const leftXLimit = (this.scene.scale.width / 2) - 450;
    const rightXLimit = (this.scene.scale.width / 2) + 450;
    const hoverLeft  = inBirdsBar && pointer.x < leftXLimit;
    const hoverRight = inBirdsBar && pointer.x > rightXLimit;

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
    const inBirdsBar = pointer.y > (this.scene.scale.height - 245);
    const { tileSize, offsetX, offsetY, gridWidth, gridHeight } = this.grid;

    this.closestCellHighlight.clear();

    if (inBirdsBar || gridX < 0 || gridX >= gridWidth || gridY < 0 || gridY >= gridHeight) {
      this.hideMergeTooltip();
      return;
    }

    const posX = offsetX + gridX * tileSize;
    const posY = offsetY + gridY * tileSize;

    if (this.activeDragTowerSourceId) {
      const targetTower = this.findTowerAt(gridX, gridY, this.activeDragTowerSourceId);
      if (targetTower && this.activeDragBirdType) {
        const resultType = getMergeResult(this.activeDragBirdType, targetTower.birdType);
        if (resultType) {
          this.closestCellHighlight.fillStyle(0x10b981, 0.5).lineStyle(4, 0x059669, 1);
          this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
          this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
          this.showMergeTooltip(targetTower, resultType);
        } else {
          this.closestCellHighlight.fillStyle(0xef4444, 0.45).lineStyle(4, 0xef4444, 1);
          this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
          this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
          this.hideMergeTooltip();
        }
      } else {
        this.closestCellHighlight.fillStyle(0xef4444, 0.35).lineStyle(3, 0xef4444, 1);
        this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
        this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
        this.hideMergeTooltip();
      }
    } else {
      const targetTower = this.findTowerAt(gridX, gridY);
      if (targetTower && this.activeDragBirdType) {
        const resultType = getMergeResult(this.activeDragBirdType, targetTower.birdType);
        if (resultType) {
          this.closestCellHighlight.fillStyle(0x10b981, 0.5).lineStyle(4, 0x059669, 1);
          this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
          this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
          this.showMergeTooltip(targetTower, resultType);
        } else {
          this.closestCellHighlight.fillStyle(0xef4444, 0.45).lineStyle(4, 0xef4444, 1);
          this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
          this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
          this.hideMergeTooltip();
        }
      } else if (this.isValidGrassTile(gridX, gridY)) {
        this.closestCellHighlight.fillStyle(0x34d399, 0.45).lineStyle(3, 0x10b981, 1);
        this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
        this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
        this.hideMergeTooltip();
      } else {
        this.closestCellHighlight.fillStyle(0xf87171, 0.35).lineStyle(3, 0xef4444, 1);
        this.closestCellHighlight.fillRect(posX, posY, tileSize, tileSize);
        this.closestCellHighlight.strokeRect(posX, posY, tileSize, tileSize);
        this.hideMergeTooltip();
      }
    }
  }

  private spawnCancelZones() {
    const width = this.scene.scale.width;
    const height = this.scene.scale.height;
    const centerX = width / 2;
    const boxY = height - 145;
    const leftX = (centerX - 450) / 2;
    const rightX = width - leftX;
    const zoneWidth = 600;
    const zoneHeight = 200;

    this.dragCancelLeftBox = this.scene.add.graphics().setDepth(30);
    this.dragCancelLeftBox.fillStyle(0xef4444, 0.6);
    this.dragCancelLeftBox.fillRoundedRect(leftX - zoneWidth / 2, boxY - zoneHeight / 2, zoneWidth, zoneHeight, 24);
    this.dragCancelLeftBox.setAlpha(0.75);
    this.dragCancelLeftText = this.scene.add.text(leftX, boxY, 'RELEASE TO CANCEL', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '30px', color: '#ffffff',
    }).setOrigin(0.5).setDepth(31);

    this.dragCancelRightBox = this.scene.add.graphics().setDepth(30);
    this.dragCancelRightBox.fillStyle(0xef4444, 0.6);
    this.dragCancelRightBox.fillRoundedRect(rightX - zoneWidth / 2, boxY - zoneHeight / 2, zoneWidth, zoneHeight, 24);
    this.dragCancelRightBox.setAlpha(0.75);
    this.dragCancelRightText = this.scene.add.text(rightX, boxY, 'RELEASE TO CANCEL', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '30px', color: '#ffffff',
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

  private drawPlacementAndMergeHighlights() {
    this.drawGrassHighlights();
    this.drawMergeHighlights();
  }

  private drawMergeHighlights() {
    if (!this.activeDragBirdType) return;
    const { tileSize, offsetX, offsetY } = this.grid;
    if (this.activeDragTowerSourceId) {
      this.gridHighlightGraphics.clear();
    }
    
    for (const otherTower of this.towers.values()) {
      if (otherTower.id === this.activeDragTowerSourceId) continue;
      
      const resultType = getMergeResult(this.activeDragBirdType, otherTower.birdType);
      if (resultType) {
        const posX = offsetX + otherTower.gridX * tileSize;
        const posY = offsetY + otherTower.gridY * tileSize;
        this.gridHighlightGraphics.fillStyle(0x3b82f6, 0.45).lineStyle(3, 0x2563eb, 1.0);
        this.gridHighlightGraphics.fillRect(posX, posY, tileSize, tileSize);
        this.gridHighlightGraphics.strokeRect(posX, posY, tileSize, tileSize);
      }
    }
  }

  private findTowerAt(gridX: number, gridY: number, excludeId?: string): Tower | null {
    for (const tower of this.towers.values()) {
      if (tower.gridX === gridX && tower.gridY === gridY && tower.id !== excludeId) {
        return tower;
      }
    }
    return null;
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

  private showMergeTooltip(targetTower: Tower, resultType: string) {
    if (this.mergeTooltip) {
      if (this.mergeTooltip.getData('targetId') === targetTower.id && this.mergeTooltip.getData('resultType') === resultType) {
        return;
      }
      this.mergeTooltip.destroy();
    }

    const x = targetTower.x;
    const y = targetTower.y;
    const stats = BIRD_STATS[resultType];
    if (!stats) return;

    const tooltip = this.scene.add.container(x, y - 180).setDepth(45);
    tooltip.setData('targetId', targetTower.id);
    tooltip.setData('resultType', resultType);

    tooltip.add(this.scene.add.nineslice(0, 0, 'box_square', undefined, 350, 220, 32, 32, 32, 32));
    const formattedName = resultType.replace(/_/g, ' ').toUpperCase();
    tooltip.add(this.scene.add.text(0, -80, `UPGRADE: ${formattedName}`, {
      fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '34px', color: stats.color,
    }).setOrigin(0.5));

    // Add respective bird head image next to stats
    const headImage = this.scene.add.image(-90, 15, `head_${resultType}`).setDisplaySize(95, 95);
    tooltip.add(headImage);

    const rows = [
      { label: 'DAMAGE',    value: String(stats.damage),  color: '#f87171' },
      { label: 'RANGE',     value: String(stats.range),   color: '#60a5fa' },
      { label: 'FIRE RATE', value: stats.fireRate,        color: '#34d399' },
      { label: 'ATTACK',    value: stats.attack,          color: '#e9d5ff' },
      { label: 'COST',      value: String(stats.cost),    color: '#fbbf24' },
    ];

    rows.forEach((row, i) => {
      const rowY = -40 + i * 25;
      tooltip.add(this.scene.add.text(-10, rowY, row.label, {
        fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '20px', color: '#94a3b8',
      }).setOrigin(0, 0.5));
      tooltip.add(this.scene.add.text(140, rowY, row.value, {
        fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '20px', color: row.color,
      }).setOrigin(1, 0.5));
    });

    if (this.activeDragTooltip) {
      this.activeDragTooltip.setVisible(false);
    }
    this.mergeTooltip = tooltip;
  }

  private hideMergeTooltip() {
    if (this.mergeTooltip) {
      this.mergeTooltip.destroy();
      this.mergeTooltip = null;
    }
    if (this.activeDragTooltip) {
      this.activeDragTooltip.setVisible(true);
    }
  }
}
