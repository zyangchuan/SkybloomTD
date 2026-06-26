import Phaser from 'phaser';
import Tower from '../components/Tower';
import { DAMAGE_TO_BIRD } from '../data/birds';

interface SmogEntry {
  sprite: Phaser.GameObjects.Sprite;
  healthBar: Phaser.GameObjects.Graphics;
  targetX: number; targetY: number;
  health: number; maxHealth: number;
}

interface ProjectileEntry {
  sprite: Phaser.GameObjects.Sprite;
  targetX: number; targetY: number; targetRotation: number;
}

export class EntitySync {
  readonly towers: Map<string, Tower> = new Map();
  activeSmogsList: Array<{ id: string, x: number, y: number, pathIndex: number }> = [];

  private smogs: Map<string, SmogEntry> = new Map();
  private smogMaxHealth: Map<string, number> = new Map();
  private projectiles: Map<string, ProjectileEntry> = new Map();

  constructor(
    private scene: Phaser.Scene,
    private tileSize: number,
    private offsetX: number,
    private offsetY: number,
  ) { }

  // ─── Sync from server state ──────────────────────────────────────────────────

  syncTowers(birdsList: any[]) {
    const activeIds = new Set<string>();
    birdsList.forEach(({ id, type, position, stats }) => {
      activeIds.add(id);
      if (!this.towers.has(id)) {
        const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
        const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
        const tower = new Tower(this.scene, posX, posY, id, type, position.x, position.y);
        if (stats?.range) tower.range = stats.range;
        tower.setScale(this.tileSize / tower.width * 1.3).setDepth(4);
        this.towers.set(id, tower);
      } else {
        const tower = this.towers.get(id)!;
        if (stats?.range) tower.range = stats.range;
      }
    });
    for (const [id, tower] of this.towers.entries()) {
      if (!activeIds.has(id)) { tower.destroy(); this.towers.delete(id); }
    }
  }

  syncSmogs(smogsList: any[]) {
    this.activeSmogsList = smogsList.map(s => ({
      id: s.id, x: s.position.x, y: s.position.y, pathIndex: s.path_index || 0,
    }));
    const activeIds = new Set<string>();

    const getEnemyBaseMultiplier = (enemyType: string) => {

      if (enemyType === "smog") {
        return { width: 1.0, height: 1.1 };
      }

      if (enemyType === "noise") {
        return { width: 1.1, height: 1.1 };
      }

      if (enemyType === "junk") {
        return { width: 1.5, height: 1.5 };
      }

      return { width: 1.0, height: 1.1 };
    }


    smogsList.forEach(({ id, type, health, position }) => {
      activeIds.add(id);
      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
      if (!this.smogMaxHealth.has(id)) this.smogMaxHealth.set(id, health);
      const maxHealth = this.smogMaxHealth.get(id) || health || 1;
      if (!this.smogs.has(id)) {
        const smog = `enemy_${type ?? "smog"}`;
        const sprite = this.scene.add.sprite(posX, posY, smog)
          .setDisplaySize(this.tileSize * getEnemyBaseMultiplier(type).width, this.tileSize * getEnemyBaseMultiplier(type).height).setDepth(15);
        const healthBar = this.scene.add.graphics().setDepth(16);
        this.smogs.set(id, { sprite, healthBar, targetX: posX, targetY: posY, health, maxHealth });
      } else {
        const e = this.smogs.get(id)!;
        e.targetX = posX; e.targetY = posY; e.health = health; e.maxHealth = maxHealth;
      }
    });
    for (const [id, e] of this.smogs.entries()) {
      if (!activeIds.has(id)) {
        e.sprite.destroy(); e.healthBar.destroy();
        this.smogs.delete(id); this.smogMaxHealth.delete(id);
      }
    }
  }

  syncProjectiles(list: any[]) {
    const activeIds = new Set<string>();
    list.forEach(({ id, damage, position, direction }) => {
      activeIds.add(id);
      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
      const textureKey = `projectile_${DAMAGE_TO_BIRD[damage] ?? 'sparrow'}`;
      let targetRotation = 0;
      if (direction && (direction.x !== 0 || direction.y !== 0)) {
        targetRotation = Math.atan2(direction.y, direction.x) + Math.PI / 2;
      }
      if (!this.projectiles.has(id)) {
        const sprite = this.scene.add.sprite(posX, posY, textureKey);
        sprite.setScale(this.tileSize / sprite.width * 0.5).setDepth(20).setRotation(targetRotation);
        this.projectiles.set(id, { sprite, targetX: posX, targetY: posY, targetRotation });
      } else {
        const e = this.projectiles.get(id)!;
        e.targetX = posX; e.targetY = posY; e.targetRotation = targetRotation;
      }
    });
    for (const [id, e] of this.projectiles.entries()) {
      if (!activeIds.has(id)) { e.sprite.destroy(); this.projectiles.delete(id); }
    }
  }

  // ─── Per-frame interpolation ─────────────────────────────────────────────────

  interpolate() {
    this.towers.forEach((tower) => tower.update());
    this.interpolateSmogs();
    this.interpolateProjectiles();
  }

  private interpolateSmogs() {
    this.smogs.forEach(({ sprite, healthBar, targetX, targetY, health, maxHealth }) => {
      sprite.x = Phaser.Math.Linear(sprite.x, targetX, 0.18);
      sprite.y = Phaser.Math.Linear(sprite.y, targetY, 0.18);

      healthBar.clear();
      const pct = Math.max(0, Math.min(1, health / maxHealth));
      const barW = 60, barH = 8;
      const barX = sprite.x - barW / 2;
      const barY = sprite.y - this.tileSize / 2 - 1;
      healthBar.fillStyle(0x1e293b, 1.0).fillRoundedRect(barX, barY, barW, barH, 3);
      if (pct > 0) {
        const color = pct < 0.3 ? 0xef4444 : pct < 0.6 ? 0xf59e0b : 0x10b981;
        healthBar.fillStyle(color, 1.0).fillRoundedRect(barX + 1, barY + 1, (barW - 2) * pct, barH - 2, 2);
      }
    });
  }

  private interpolateProjectiles() {
    this.projectiles.forEach(({ sprite, targetX, targetY, targetRotation }) => {
      sprite.x = Phaser.Math.Linear(sprite.x, targetX, 0.28);
      sprite.y = Phaser.Math.Linear(sprite.y, targetY, 0.28);
      sprite.rotation = Phaser.Math.Angle.RotateTo(sprite.rotation, targetRotation, 0.25);
    });
  }

  // ─── Cleanup ─────────────────────────────────────────────────────────────────

  destroy() {
    this.towers.forEach((t) => t.destroy());
    this.towers.clear();
    this.smogs.forEach((s) => { s.sprite.destroy(); s.healthBar.destroy(); });
    this.smogs.clear();
    this.smogMaxHealth.clear();
    this.projectiles.forEach((p) => p.sprite.destroy());
    this.projectiles.clear();
  }
}
