import Phaser from 'phaser';
import Tower from '../components/Tower';
import { BIRD_STATS, EVOLVED_BIRD_STATS, BirdStats } from '../data/birds';

export class TowerUpgradePanel {
  private backdrop: Phaser.GameObjects.Graphics | null = null;
  private container: Phaser.GameObjects.Container | null = null;

  constructor(
    private scene: Phaser.Scene,
    private onEvolve: (towerId: string, birdType: string) => void,
    private getEssence: () => number,
  ) {}

  show(tower: Tower) {
    this.hide();

    // Dimming backdrop — clicking outside closes panel
    this.backdrop = this.scene.add.graphics().setDepth(100);
    this.backdrop.fillStyle(0x000000, 0.6).fillRect(-2000, -2000, 6000, 5000);
    this.backdrop.setInteractive(
      new Phaser.Geom.Rectangle(-2000, -2000, 6000, 5000),
      Phaser.Geom.Rectangle.Contains
    );
    this.backdrop.on('pointerdown', () => this.hide());

    const c = this.scene.add.container(tower.x, tower.y).setDepth(110);

    // Panel card — interactive so clicks don't fall through to backdrop
    const panelBg = this.scene.add.nineslice(0, 0, 'box_orange_square', undefined, 550, 550, 64, 64, 64, 64);
    panelBg.setInteractive();
    c.add(panelBg);

    const baseBirdType = tower.birdType.startsWith('evolve_')
      ? tower.birdType.replace('evolve_', '')
      : tower.birdType;
    const baseStats = BIRD_STATS[baseBirdType];
    const evoStats  = EVOLVED_BIRD_STATS[baseBirdType];

    // ── X close button ───────────────────────────────────────────────────────
    const closeBtn = this.scene.add.text(220, -218, 'X', {
      fontFamily: 'Concert One', fontSize: '30px', color: '#451a03',
    }).setOrigin(0.5).setInteractive({ useHandCursor: true });
    closeBtn.on('pointerover', () => closeBtn.setColor('#ef4444'));
    closeBtn.on('pointerout',  () => closeBtn.setColor('#451a03'));
    closeBtn.on('pointerdown', () => this.hide());
    c.add(closeBtn);

    // ── Bird name ────────────────────────────────────────────────────────────
    c.add(this.scene.add.text(0, -230, baseBirdType.toUpperCase(), {
      fontFamily: 'Concert One',
      fontSize: '50px', color: '#070505',
    }).setOrigin(0.5));

    // ── Head image ───────────────────────────────────────────────────────────
    const headKey = tower.evolveLevel > 0 ? `head_evolve_${baseBirdType}` : `head_${baseBirdType}`;
    c.add(this.scene.add.image(0, -130, headKey).setDisplaySize(150, 150));

    // ── Stats / buttons ──────────────────────────────────────────────────────
    if (tower.evolveLevel === 0 && baseStats && evoStats) {
      this.buildUpgradeView(c, tower, baseBirdType, baseStats, evoStats);
    } else {
      this.buildMaxLevelView(c, evoStats ?? baseStats);
    }

    this.container = c;
  }

  private buildUpgradeView(
    c: Phaser.GameObjects.Container,
    tower: Tower,
    baseBirdType: string,
    baseStats: BirdStats,
    evoStats: BirdStats,
  ) {
    // Column headers
    c.add(this.scene.add.text(-60, -45, 'CURRENT', {
      fontFamily: 'Concert One', fontSize: '30px', color: '#94a3b8',
    }).setOrigin(0.5));
    c.add(this.scene.add.text(130, -45, 'EVOLVED', {
      fontFamily: 'Concert One', fontSize: '30px', color: '#4ade80',
    }).setOrigin(0.5));

    const rows = [
      { label: 'DAMAGE',    from: String(baseStats.damage),    to: String(evoStats.damage) },
      { label: 'RANGE',     from: String(baseStats.range),     to: String(evoStats.range) },
      { label: 'FIRE RATE', from: baseStats.fireRate,           to: evoStats.fireRate },
    ];
    rows.forEach((row, i) => {
      const y = 14 + i * 38;
      c.add(this.scene.add.text(-230, y, row.label, {
        fontFamily: 'Concert One', fontSize: '25px', color: '#cbd5e1',
      }).setOrigin(0, 0.5));
      c.add(this.scene.add.text(-60, y, row.from, {
        fontFamily: 'Concert One', fontSize: '25px', color: '#ffffff',
      }).setOrigin(0.5, 0.5));
      c.add(this.scene.add.text(30, y, '->', {
        fontFamily: 'Concert One', fontSize: '25px', color: '#64748b',
      }).setOrigin(0, 0.5));
      c.add(this.scene.add.text(130, y, row.to, {
        fontFamily: 'Concert One', fontSize: '25px', color: '#4ade80',
      }).setOrigin(0.5, 0.5));
    });

    // ── Essence cost ─────────────────────────────────────────────────────────
    const cost      = baseStats.cost;
    const canAfford = this.getEssence() >= cost;
    c.add(this.scene.add.text(0, 122, `COST: ${cost} ESSENCE`, {
      fontFamily: 'Concert One',
      fontSize: '20px', color: canAfford ? '#fbbf24' : '#ef4444',
    }).setOrigin(0.5));

    // ── EVOLVE button ─────────────────────────────────────────────────────────
    if (canAfford) {
      const evoBtn   = this.scene.add.sprite(0, 195, 'btn_blue_round').setScale(1.2).setInteractive({ useHandCursor: true });
      const evoLabel = this.scene.add.text(0, 195, 'EVOLVE', {
        fontFamily: 'Concert One', fontSize: '20px', color: '#ffffff',
      }).setOrigin(0.5);
      evoBtn.on('pointerover', () => { evoBtn.setScale(1.3); evoLabel.setColor('#fef3c7'); });
      evoBtn.on('pointerout',  () => { evoBtn.setScale(1.2); evoLabel.setColor('#ffffff'); });
      evoBtn.on('pointerdown', () => {
        tower.evolve();                          // optimistic visual update
        this.onEvolve(tower.id, baseBirdType);   // send to server
        this.hide();
      });
      c.add([evoBtn, evoLabel]);
    } else {
      const evoBtn   = this.scene.add.sprite(0, 195, 'btn_blank_round').setScale(1.2).setTint(0x888888);
      const evoLabel = this.scene.add.text(0, 195, 'NOT ENOUGH ESSENCE', {
        fontFamily: 'Concert One', fontSize: '20px', color: '#64748b',
      }).setOrigin(0.5);
      c.add([evoBtn, evoLabel]);
    }
  }

  private buildMaxLevelView(c: Phaser.GameObjects.Container, stats: BirdStats | undefined) {
    c.add(this.scene.add.text(0, -30, 'MAX LEVEL', {
      fontFamily: 'Concert One', fontSize: '35px', color: '#fbbf24',
    }).setOrigin(0.5));

    if (!stats) return;
    const rows = [
      { label: 'DAMAGE',    value: String(stats.damage) },
      { label: 'RANGE',     value: String(stats.range) },
      { label: 'FIRE RATE', value: stats.fireRate },
    ];
    rows.forEach((row, i) => {
      const y = 24 + i * 38;
      c.add(this.scene.add.text(-180, y, row.label, {
        fontFamily: 'Concert One', fontSize: '30px', color: '#94a3b8',
      }).setOrigin(0, 0.5));
      c.add(this.scene.add.text(180, y, row.value, {
        fontFamily: 'Concert One', fontSize: '30px', color: '#fbbf24',
      }).setOrigin(1, 0.5));
    });
  }

  hide() {
    this.backdrop?.destroy();  this.backdrop  = null;
    this.container?.destroy(); this.container = null;
  }

  destroy() { this.hide(); }
}
