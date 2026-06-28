import Phaser from 'phaser';
import Tower from '../components/Tower';
import { BIRD_STATS } from '../data/birds';

interface EnemyEntry {
  sprite: Phaser.GameObjects.Sprite;
  healthBar: Phaser.GameObjects.Graphics;
  targetX: number; targetY: number;
  health: number; maxHealth: number;
  type: string;
  isDying?: boolean;
}

interface ProjectileEntry {
  sprite: Phaser.GameObjects.Sprite;
  targetX: number; targetY: number; targetRotation: number;
}

export class EntitySync {
  readonly towers: Map<string, Tower> = new Map();
  activeEnemiesList: Array<{ id: string, x: number, y: number, pathIndex: number }> = [];

  private enemies: Map<string, EnemyEntry> = new Map();
  private enemyMaxHealth: Map<string, number> = new Map();
  private projectiles: Map<string, ProjectileEntry> = new Map();

  private clientProjectiles: Array<{
    sprite: Phaser.GameObjects.Sprite;
    targetId: string;
    lastTargetX: number;
    lastTargetY: number;
    speed: number;
    birdType: string;
    towerX: number;
    towerY: number;
    angleOffset: number;
  }> = [];

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
        const scaleMultiplier = type === 'sun_god' ? 2.0 : 1.2;
        tower.setScale(this.tileSize / tower.width * scaleMultiplier).setDepth(4);
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

  syncEnemies(enemiesList: any[]) {
    this.activeEnemiesList = enemiesList.map(s => ({
      id: s.id, x: s.position.x, y: s.position.y, pathIndex: s.path_index || 0,
    }));
    const activeIds = new Set<string>();

    const getEnemyBaseMultiplier = (enemyType: string) => {
      if (enemyType === "smog") {
        return { width: 1.0, height: 1.0 };
      }
      if (enemyType === "noise") {
        return { width: 1.0, height: 1.0 };
      }
      if (enemyType === "junk") {
        return { width: 1.5, height: 1.5 };
      }
      return { width: 1.0, height: 1.0 };
    }

    enemiesList.forEach(({ id, type, health, position }) => {
      activeIds.add(id);
      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
      if (!this.enemyMaxHealth.has(id)) this.enemyMaxHealth.set(id, health);
      const maxHealth = this.enemyMaxHealth.get(id) || health || 1;
      if (!this.enemies.has(id)) {
        const enemyTexture = `enemy_${type ?? "smog"}`;
        const sprite = this.scene.add.sprite(posX, posY, enemyTexture)
          .setDisplaySize(this.tileSize * getEnemyBaseMultiplier(type).width, this.tileSize * getEnemyBaseMultiplier(type).height).setDepth(15);
        const healthBar = this.scene.add.graphics().setDepth(16);
        this.enemies.set(id, { sprite, healthBar, targetX: posX, targetY: posY, health, maxHealth, type });
      } else {
        const e = this.enemies.get(id)!;
        e.targetX = posX; e.targetY = posY; e.health = health; e.maxHealth = maxHealth;
      }
    });
    for (const [id, e] of this.enemies.entries()) {
      if (!activeIds.has(id)) {
        if (e.isDying) {
          continue;
        }
        const hasProjectiles = this.clientProjectiles.some(p => p.targetId === id);
        if (hasProjectiles) {
          e.isDying = true;
          e.health = 0;
          e.healthBar.clear();
        } else {
          e.sprite.destroy(); e.healthBar.destroy();
          this.enemies.delete(id); this.enemyMaxHealth.delete(id);
        }
      }
    }
  }

  syncProjectiles(_list: any[]) {
    // Deprecated: client now simulates projectiles locally
  }

  // ─── Event Processing ────────────────────────────────────────────────────────

  processEvents(events: any[]) {
    events.forEach(event => {
      if (event.type === 'bird.attack') {
        this.spawnClientProjectile(event.bird_id, event.enemy_id);
      }
    });
  }

  private spawnClientProjectile(birdId: string, enemyId: string) {
    const sourceTower = this.towers.get(birdId);
    if (!sourceTower) return;

    const targetEnemy = this.enemies.get(enemyId);
    const lastX = targetEnemy ? targetEnemy.sprite.x : sourceTower.x;
    const lastY = targetEnemy ? targetEnemy.sprite.y : sourceTower.y;

    const birdType = sourceTower.birdType;
    const stats = BIRD_STATS[birdType];
    const projectileScale = birdType === 'sun_god' ? 1.0 : 0.5;

    const createSingleProjectile = (angleOffset: number) => {
      const sprite = this.scene.add.sprite(sourceTower.x, sourceTower.y, `projectile_${birdType}`);
      sprite.setScale(this.tileSize / sprite.width * projectileScale).setDepth(20);

      this.clientProjectiles.push({
        sprite,
        targetId: enemyId,
        lastTargetX: lastX,
        lastTargetY: lastY,
        speed: 15 * this.tileSize,
        birdType,
        towerX: sourceTower.x,
        towerY: sourceTower.y,
        angleOffset,
      });
    };

    if (stats && stats.attack === 'SPLASH') {
      // Spawn 3 projectiles fanning out to cover range of attack
      const offsets = [-0.22, 0, 0.22]; // angle offsets in radians (~12.5 degrees)
      offsets.forEach(offset => createSingleProjectile(offset));
    } else {
      createSingleProjectile(0);
    }
  }

  // ─── Per-frame interpolation & simulation ────────────────────────────────────

  interpolate(deltaMs: number = 16.66) {
    this.towers.forEach((tower) => tower.update());
    this.interpolateEnemies();
    this.updateClientProjectiles(deltaMs);
  }

  private interpolateEnemies() {
    this.enemies.forEach((e) => {
      if (e.isDying) {
        e.sprite.setAlpha(Math.max(0, e.sprite.alpha - 0.05));
        e.healthBar.clear();
        return;
      }
      e.sprite.x = Phaser.Math.Linear(e.sprite.x, e.targetX, 0.18);
      e.sprite.y = Phaser.Math.Linear(e.sprite.y, e.targetY, 0.18);

      e.healthBar.clear();
      const pct = Math.max(0, Math.min(1, e.health / e.maxHealth));
      const barW = 60, barH = 8;
      const barX = e.sprite.x - barW / 2;
      let barY = e.sprite.y - this.tileSize / 2 - 1;
      if (e.type == "junk") {
        barY = e.sprite.y - this.tileSize / 2 - 10;
      }
      e.healthBar.fillStyle(0x1e293b, 1.0).fillRoundedRect(barX, barY, barW, barH, 3);
      if (pct > 0) {
        const color = pct < 0.3 ? 0xef4444 : pct < 0.6 ? 0xf59e0b : 0x10b981;
        e.healthBar.fillStyle(color, 1.0).fillRoundedRect(barX + 1, barY + 1, (barW - 2) * pct, barH - 2, 2);
      }
    });
  }

  private updateClientProjectiles(deltaMs: number) {
    const dt = deltaMs / 1000;
    const active: any[] = [];

    this.clientProjectiles.forEach((p) => {
      let tx = p.lastTargetX;
      let ty = p.lastTargetY;

      const currentTarget = this.enemies.get(p.targetId);
      if (currentTarget && currentTarget.sprite && currentTarget.sprite.active) {
        tx = currentTarget.sprite.x;
        ty = currentTarget.sprite.y;
        p.lastTargetX = tx;
        p.lastTargetY = ty;
      }

      // Fan out tracking positions relative to the source tower
      if (p.angleOffset !== 0) {
        const dx = tx - p.towerX;
        const dy = ty - p.towerY;
        const cos = Math.cos(p.angleOffset);
        const sin = Math.sin(p.angleOffset);
        tx = p.towerX + dx * cos - dy * sin;
        ty = p.towerY + dx * sin + dy * cos;
      }

      const dx = tx - p.sprite.x;
      const dy = ty - p.sprite.y;
      const dist = Math.sqrt(dx * dx + dy * dy);

      if (dist > 0) {
        p.sprite.rotation = Math.atan2(dy, dx) + Math.PI / 2;
      }

      const moveStep = p.speed * dt;

      if (dist <= moveStep) {
        p.sprite.destroy();
        const targetEnemy = this.enemies.get(p.targetId);
        if (targetEnemy && targetEnemy.isDying) {
          const otherProjectiles = this.clientProjectiles.some(other => other !== p && other.targetId === p.targetId);
          if (!otherProjectiles) {
            targetEnemy.sprite.destroy();
            targetEnemy.healthBar.destroy();
            this.enemies.delete(p.targetId);
            this.enemyMaxHealth.delete(p.targetId);
          }
        }
      } else {
        p.sprite.x += (dx / dist) * moveStep;
        p.sprite.y += (dy / dist) * moveStep;
        active.push(p);
      }
    });

    this.clientProjectiles = active;
  }

  // ─── Cleanup ─────────────────────────────────────────────────────────────────

  destroy() {
    this.towers.forEach((t) => t.destroy());
    this.towers.clear();
    this.enemies.forEach((s) => { s.sprite.destroy(); s.healthBar.destroy(); });
    this.enemies.clear();
    this.enemyMaxHealth.clear();
    this.projectiles.forEach((p) => p.sprite.destroy());
    this.projectiles.clear();
    this.clientProjectiles.forEach((p) => p.sprite.destroy());
    this.clientProjectiles = [];
  }
}
