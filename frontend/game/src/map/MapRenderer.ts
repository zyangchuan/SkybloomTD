import Phaser from 'phaser';

export class MapRenderer {
  constructor(
    private scene: Phaser.Scene,
    private tileSize: number,
    private offsetX: number,
    private offsetY: number,
    private gridWidth: number,
    private gridHeight: number,
  ) {}

  render(mapData: any) {
    this.renderGrass();
    this.renderPaths(mapData.enemy_path || []);
    this.renderObstacles(mapData.objects || []);
  }

  private renderGrass() {
    for (let y = 0; y < this.gridHeight; y++) {
      for (let x = 0; x < this.gridWidth; x++) {
        this.scene.add.image(
          this.offsetX + x * this.tileSize + this.tileSize / 2,
          this.offsetY + y * this.tileSize + this.tileSize / 2,
          'grass'
        ).setDisplaySize(this.tileSize, this.tileSize);
      }
    }
  }

  private renderPaths(path: any[]) {
    path.forEach((tile: any) => {
      this.scene.add.image(
        this.offsetX + tile.x * this.tileSize + this.tileSize / 2,
        this.offsetY + tile.y * this.tileSize + this.tileSize / 2,
        this.getPathSpriteKey(tile)
      ).setDisplaySize(this.tileSize, this.tileSize);
    });
  }

  private renderObstacles(objects: any[]) {
    objects.sort((a: any, b: any) => a.y - b.y);
    objects.forEach((obj: any) => {
      const spriteKey = this.getObjectSpriteKey(obj);
      const img = this.scene.add.image(
        this.offsetX + obj.x * this.tileSize + this.tileSize / 2,
        this.offsetY + obj.y * this.tileSize + this.tileSize / 2,
        spriteKey
      );
      img.setScale(this.getObstacleScale(obj, spriteKey, img)).setDepth(1);
    });
  }

  private getObstacleScale(obj: any, spriteKey: string, img: Phaser.GameObjects.Image): number {
    if (obj.type === 'tree')       return (this.tileSize * 1.85) / img.height;
    if (obj.type === 'bush')       return (this.tileSize * 0.85) / img.width;
    if (obj.type === 'tree_stump') return (this.tileSize * 0.80) / img.width;
    if (obj.type === 'rock') {
      const factor = (spriteKey === 'rock_04' || spriteKey === 'rock_05') ? 0.38 : 0.85;
      return (this.tileSize * factor) / img.width;
    }
    return this.tileSize / img.height;
  }

  private getPathSpriteKey(tile: any): string {
    const { kind, from, to } = tile;
    if (kind === 'straight') return tile.axis === 'vertical' ? 'path_vert' : 'path_horiz';
    if (kind === 'start')    return (to === 'north'   || to === 'south')   ? 'path_vert' : 'path_horiz';
    if (kind === 'end')      return (from === 'north' || from === 'south') ? 'path_vert' : 'path_horiz';
    if (kind === 'turn') {
      const dirs = new Set([from, to]);
      if (dirs.has('north') && dirs.has('east'))  return 'path_corner_ne';
      if (dirs.has('south') && dirs.has('east'))  return 'path_corner_se';
      if (dirs.has('south') && dirs.has('west'))  return 'path_corner_sw';
      if (dirs.has('west')  && dirs.has('north')) return 'path_corner_wn';
    }
    return 'path_horiz';
  }

  private getObjectSpriteKey(obj: any): string {
    const hash = obj.x * 7 + obj.y * 13;
    if (obj.type === 'tree')       return `tree_0${(hash % 3) + 1}`;
    if (obj.type === 'tree_stump') return `tree_stump_0${(hash % 2) + 1}`;
    if (obj.type === 'bush')       return `bush_0${(hash % 3) + 1}`;
    if (obj.type === 'rock')       return `rock_0${(hash % 5) + 1}`;
    return 'tree_01';
  }
}
