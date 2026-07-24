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
    isDirectional?: boolean;
    dirX?: number;
    dirY?: number;
    maxRange?: number;
    distanceTraveled?: number;
    glowCircle?: Phaser.GameObjects.Image;
  }> = [];

  constructor(
    private scene: Phaser.Scene,
    private tileSize: number,
    private offsetX: number,
    private offsetY: number,
  ) { }

  syncTowers(birdsList: any[]) {
    const activeIds = new Set<string>();
    birdsList.forEach(({ id, type, position, stats }) => {
      activeIds.add(id);
      if (!this.towers.has(id)) {
        const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
        const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
        const tower = new Tower(this.scene, posX, posY, id, type, position.x, position.y);
        if (stats?.range) tower.range = stats.range;
        if (stats?.lifespan) tower.lifespan = stats.lifespan;
        if (stats?.spread) tower.spread = stats.spread;
        let scaleMultiplier = 1.2;
        if (type === 'sun_god') {
          scaleMultiplier = 2.0;
        } else if (type === 'phoenix') {
          scaleMultiplier = 1.8;
        } else if (type === 'kingfisher' || type === 'falcon') {
          scaleMultiplier = 1.6;
        }
        tower.setScale(this.tileSize / tower.width * scaleMultiplier).setDepth(4);
        this.towers.set(id, tower);
      } else {
        const tower = this.towers.get(id)!;
        if (stats?.range) tower.range = stats.range;
        if (stats?.lifespan) tower.lifespan = stats.lifespan;
        if (stats?.spread) tower.spread = stats.spread;
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

    const getEnemyDepth = (enemyType: string) => enemyType === "junk" ? 17 : 15;

    enemiesList.forEach(({ id, type, health, position }) => {
      activeIds.add(id);
      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;
      if (!this.enemyMaxHealth.has(id)) this.enemyMaxHealth.set(id, health);
      const maxHealth = this.enemyMaxHealth.get(id) || health || 1;
      if (!this.enemies.has(id)) {
        const enemyTexture = `enemy_${type ?? "smog"}`;
        const enemyDepth = getEnemyDepth(type);
        const sprite = this.scene.add.sprite(posX, posY, enemyTexture)
          .setDisplaySize(this.tileSize * getEnemyBaseMultiplier(type).width, this.tileSize * getEnemyBaseMultiplier(type).height).setDepth(enemyDepth);
        const healthBar = this.scene.add.graphics().setDepth(enemyDepth + 1);
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

    if (birdType === 'phoenix') {
      const numDirections = 12;
      const range = sourceTower.range * this.tileSize * 0.85;

      // expanding outer red blast circle
      const blast = this.scene.add.circle(sourceTower.x, sourceTower.y, 10, 0xef4444, 0.35).setDepth(19);
      this.scene.tweens.add({
        targets: blast,
        radius: range,
        alpha: 0,
        duration: 250,
        ease: 'Quad.easeOut',
        onComplete: () => blast.destroy()
      });

      // expanding inner orange blast core
      const blastCore = this.scene.add.circle(sourceTower.x, sourceTower.y, 5, 0xf97316, 0.5).setDepth(19);
      this.scene.tweens.add({
        targets: blastCore,
        radius: range * 0.7,
        alpha: 0,
        duration: 150,
        ease: 'Quad.easeOut',
        onComplete: () => blastCore.destroy()
      });



      if (!this.scene.anims.exists('phoenix_fireball_fly')) {
        this.scene.anims.create({
          key: 'phoenix_fireball_fly',
          frames: this.scene.anims.generateFrameNumbers('projectile_phoenix', { start: 0, end: 7 }),
          frameRate: 15,
          repeat: -1
        });
      }

      if (!this.scene.textures.exists('soft_glow')) {
        const canvas = this.scene.textures.createCanvas('soft_glow', 64, 64);
        if (canvas) {
          const ctx = canvas.context;
          const grad = ctx.createRadialGradient(32, 32, 0, 32, 32, 32);
          grad.addColorStop(0, 'rgba(249, 115, 22, 1)'); // Orange
          grad.addColorStop(1, 'rgba(249, 115, 22, 0)');
          ctx.fillStyle = grad;
          ctx.fillRect(0, 0, 64, 64);
          canvas.refresh();
        }
      }

      for (let i = 0; i < numDirections; i++) {
        const angle = (i * 2 * Math.PI) / numDirections;
        const dirX = Math.cos(angle);
        const dirY = Math.sin(angle);

        const sprite = this.scene.add.sprite(sourceTower.x, sourceTower.y, 'projectile_phoenix');
        // Note: sprite.width will be 640, since the spritesheet is loaded with frameWidth: 640
        sprite.setScale((this.tileSize / 640) * 2.1).setDepth(20);
        sprite.rotation = angle + Math.PI; // Face the direction of travel (offset by 180 degrees)
        sprite.play('phoenix_fireball_fly');

        // Create a soft feathered radial glow image behind the fireball
        const glowCircle = this.scene.add.image(sourceTower.x, sourceTower.y, 'soft_glow')
          .setDepth(19)
          .setAlpha(0.6)
          .setDisplaySize(this.tileSize * 2.4, this.tileSize * 2.4);

        this.clientProjectiles.push({
          sprite,
          glowCircle,
          targetId: '',
          lastTargetX: sourceTower.x,
          lastTargetY: sourceTower.y,
          speed: 14 * this.tileSize, // Fireballs travel at a nice visible speed
          birdType,
          towerX: sourceTower.x,
          towerY: sourceTower.y,
          angleOffset: 0,
          isDirectional: true,
          dirX,
          dirY,
          maxRange: range,
          distanceTraveled: 0,
        });
      }
      return;
    }

    const stats = BIRD_STATS[birdType];
    const projectileScale = birdType === 'sun_god' ? 2.2 : 0.5;

    if (birdType === 'sun_god') {
      if (!this.scene.anims.exists('sun_god_arrow_fly')) {
        this.scene.anims.create({
          key: 'sun_god_arrow_fly',
          frames: this.scene.anims.generateFrameNumbers('projectile_sun_god', { start: 0, end: 7 }),
          frameRate: 15,
          repeat: -1
        });
      }
      if (!this.scene.textures.exists('yellow_glow')) {
        const canvas = this.scene.textures.createCanvas('yellow_glow', 64, 64);
        if (canvas) {
          const ctx = canvas.context;
          const grad = ctx.createRadialGradient(32, 32, 0, 32, 32, 32);
          grad.addColorStop(0, 'rgba(254, 240, 138, 1)'); // Yellow
          grad.addColorStop(1, 'rgba(254, 240, 138, 0)');
          ctx.fillStyle = grad;
          ctx.fillRect(0, 0, 64, 64);
          canvas.refresh();
        }
      }
    }

    const createSingleProjectile = (angleOffset: number) => {
      const sprite = this.scene.add.sprite(sourceTower.x, sourceTower.y, `projectile_${birdType}`);
      const baseWidth = birdType === 'sun_god' ? 480 : sprite.width;
      sprite.setScale(this.tileSize / baseWidth * projectileScale).setDepth(20);

      let glowCircle: Phaser.GameObjects.Image | undefined;
      if (birdType === 'sun_god') {
        sprite.setTintFill(0xfffde7); // Fully solid light yellow arrow
        sprite.play('sun_god_arrow_fly');
        glowCircle = this.scene.add.image(sourceTower.x, sourceTower.y, 'yellow_glow')
          .setDepth(19)
          .setAlpha(0.65)
          .setDisplaySize(this.tileSize * 1.5, this.tileSize * 1.5);
      }

      const hasLifespan = !!(sourceTower.lifespan || stats?.lifespan);
      const targetAngle = Math.atan2(lastY - sourceTower.y, lastX - sourceTower.x) + angleOffset;
      const dirX = Math.cos(targetAngle);
      const dirY = Math.sin(targetAngle);

      if (hasLifespan) {
        if (birdType === 'sun_god') {
          sprite.rotation = targetAngle + Math.PI;
        } else {
          sprite.rotation = targetAngle + Math.PI / 2;
        }
      }

      this.clientProjectiles.push({
        sprite,
        glowCircle,
        targetId: enemyId,
        lastTargetX: lastX,
        lastTargetY: lastY,
        speed: 28 * this.tileSize,
        birdType,
        towerX: sourceTower.x,
        towerY: sourceTower.y,
        angleOffset,
        maxRange: (sourceTower.lifespan || sourceTower.range || (stats?.lifespan) || (stats?.range) || 3.0) * this.tileSize,
        distanceTraveled: 0,
        isDirectional: hasLifespan,
        dirX,
        dirY,
      });
    };

    if (stats && stats.attack === 'SPLASH') {
      const spread = sourceTower.spread || stats.spread || (stats as any)?.Spread || (Math.PI / 12);
      const offsets = [-spread, 0, spread];
      offsets.forEach(offset => createSingleProjectile(offset));
    } else {
      createSingleProjectile(0);
    }
  }

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
      if (p.isDirectional) {
        const moveStep = p.speed * dt;
        p.distanceTraveled = (p.distanceTraveled || 0) + moveStep;

        if (p.distanceTraveled >= (p.maxRange || 0)) {
          p.sprite.destroy();
          if (p.glowCircle) p.glowCircle.destroy();
        } else {
          p.sprite.x += (p.dirX || 0) * moveStep;
          p.sprite.y += (p.dirY || 0) * moveStep;

          if (p.glowCircle) {
            p.glowCircle.x = p.sprite.x;
            p.glowCircle.y = p.sprite.y;
          }

          // Fade out near the end of range (last 20%)
          const remainingPct = 1 - (p.distanceTraveled / (p.maxRange || 1));
          if (remainingPct < 0.2) {
            const alpha = remainingPct / 0.2;
            p.sprite.setAlpha(alpha);
            if (p.glowCircle) p.glowCircle.setAlpha(alpha * 0.6);
          }
          active.push(p);
        }
        return;
      }

      let tx = p.lastTargetX;
      let ty = p.lastTargetY;

      const currentTarget = this.enemies.get(p.targetId);
      if (currentTarget && currentTarget.sprite && currentTarget.sprite.active) {
        tx = currentTarget.sprite.x;
        ty = currentTarget.sprite.y;
        p.lastTargetX = tx;
        p.lastTargetY = ty;
      }

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
        if (p.birdType === 'sun_god') {
          p.sprite.rotation = Math.atan2(dy, dx) + Math.PI;
        } else {
          p.sprite.rotation = Math.atan2(dy, dx) + Math.PI / 2;
        }
      }

      const moveStep = p.speed * dt;

      // Track distance traveled for normal projectiles
      p.distanceTraveled = (p.distanceTraveled || 0) + moveStep;

      if (p.maxRange && p.distanceTraveled >= p.maxRange) {
        p.sprite.destroy();
        if (p.glowCircle) p.glowCircle.destroy();
        return;
      }

      if (dist <= moveStep) {
        p.sprite.destroy();
        if (p.glowCircle) p.glowCircle.destroy();
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
        if (p.glowCircle) {
          p.glowCircle.x = p.sprite.x;
          p.glowCircle.y = p.sprite.y;
        }
        active.push(p);
      }
    });

    this.clientProjectiles = active;
  }

  destroy() {
    this.towers.forEach((t) => t.destroy());
    this.towers.clear();
    this.enemies.forEach((s) => { s.sprite.destroy(); s.healthBar.destroy(); });
    this.enemies.clear();
    this.enemyMaxHealth.clear();
    this.projectiles.forEach((p) => p.sprite.destroy());
    this.projectiles.clear();
    this.clientProjectiles.forEach((p) => {
      p.sprite.destroy();
      if (p.glowCircle) p.glowCircle.destroy();
    });
    this.clientProjectiles = [];
  }
}
