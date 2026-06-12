import Phaser from 'phaser';
import { MapRenderer } from '../map/MapRenderer';
import { GameHUD } from '../ui/GameHUD';
import { BirdTray } from '../ui/BirdTray';
import { QuizManager } from '../ui/QuizManager';
import { GameOverlay } from '../ui/GameOverlay';
import { TowerUpgradePanel } from '../ui/TowerUpgradePanel';
import { DragController } from '../input/DragController';
import { EntitySync } from '../game/EntitySync';
import { BgmManager } from '../audio/BgmManager';

export default class GameScene extends Phaser.Scene {
  private ws: WebSocket | null = null;
  private levelId = '';
  private sessionId = '';
  private currentEssence = 0;
  private currentWave: number = 0;
  private quizOpen = false;

  // Grid params
  private tileSize = 0;
  private offsetX = 0;
  private offsetY = 0;
  private gridWidth = 18;
  private gridHeight = 12;
  private enemyPath: any[] = [];
  private obstacles: any[] = [];

  // Sub-systems
  private hud!: GameHUD;
  private quiz!: QuizManager;
  private overlay!: GameOverlay;
  private upgradePanel!: TowerUpgradePanel;
  private drag!: DragController;
  private entities!: EntitySync;
  private bgm!: BgmManager;

  constructor() { super('GameScene'); }

  // ─── Lifecycle ───────────────────────────────────────────────────────────────
  create(data: { initialState: any, ws: WebSocket, levelId: string }) {
    this.ws = data.ws;
    this.levelId = data.levelId;
    this.bgm = new BgmManager(this);

    // Update bgm based on wave changes, which are part of the state sent by the server
    this.sound.pauseOnBlur = false; // Keep music playing even if the game loses focus
    this.bgm.updateForWave(1); // Start with first wave bgm on first interaction


    const mapData = data.initialState?.map;
    if (!mapData) { console.error('No map data in initial state.'); return; }

    this.initGrid(mapData);
    new MapRenderer(this, this.tileSize, this.offsetX, this.offsetY, this.gridWidth, this.gridHeight).render(mapData);
    this.entities = new EntitySync(this, this.tileSize, this.offsetX, this.offsetY);
    this.overlay = new GameOverlay(this, (t, d) => this.sendWs(t, d), () => this.sessionId, () => this.levelId, () => this.quiz.clear(), (v) => this.bgm.setVolume(v), this.ws);
    this.events.on('game.resumed', () => this.bgm.resume());
    this.upgradePanel = new TowerUpgradePanel(
      this,
      (towerId, birdType) => this.sendWs('game.action.evolve_tower', { tower_id: towerId, bird_type: birdType }),
      () => this.currentEssence,
    );
    this.hud = new GameHUD(this, () => { this.upgradePanel.hide(); this.overlay.showPauseWindow(); this.bgm.pause(); });
    this.quiz = new QuizManager(this, this.ws, 
      () => { this.quizOpen = true;
              this.tweens.pauseAll();
              this.anims.pauseAll();
              this.sendWs('game.pause');
              this.bgm.pause(); },
      () => { this.quizOpen = false;
              this.tweens.resumeAll();
              this.anims.resumeAll();
              this.sendWs('game.resume');
              this.bgm.resume(); }
      );
 

    this.quiz.createHUD();


    this.drag = new DragController(
      this,
      (birdType, x, y) => this.sendWs('game.action.place_tower', { bird_type: birdType, x, y }),
      { tileSize: this.tileSize, offsetX: this.offsetX, offsetY: this.offsetY, gridWidth: this.gridWidth, gridHeight: this.gridHeight },
      this.enemyPath, this.obstacles, this.entities.towers,
    );
    this.drag.setup();

    this.entities.onTowerClick = (tower) => {
      if (!this.overlay.isPaused() && !this.drag.isDragging()) {
        this.upgradePanel.show(tower);
      }
    };

    new BirdTray(
      this,
      () => this.drag.isDragging(),
      (birdType) => [...this.entities.towers.values()].some(t => t.birdType === `evolve_${birdType}`),
    );
    this.setupWebSocket(data.levelId);
    this.setupShutdown();
  }

  update() {
    if (this.overlay.isPaused() ||  this.quizOpen) return;
    this.entities.interpolate();
  }

  // ─── Grid ────────────────────────────────────────────────────────────────────

  private initGrid(mapData: any) {
    this.gridWidth = mapData.width || 18;
    this.gridHeight = mapData.height || 12;
    this.tileSize = Math.floor(Math.min(this.scale.width / this.gridWidth, this.scale.height / this.gridHeight));
    this.offsetX = (this.scale.width - this.gridWidth * this.tileSize) / 2;
    this.offsetY = (this.scale.height - this.gridHeight * this.tileSize) / 2;
    this.enemyPath = mapData.enemy_path || [];
    this.obstacles = mapData.objects || [];
  }

  // ─── WebSocket ───────────────────────────────────────────────────────────────

  private setupWebSocket(levelId: string) {
    if (!this.ws) return;
    this.ws.onmessage = null;
    this.ws.onmessage = (event) => {
      try { this.handleServerMessage(JSON.parse(event.data)); }
      catch (err) { console.error('Failed to parse WebSocket message:', err); }
    };
    this.ws.send(JSON.stringify({ type: 'game.session.start', data: { level_id: levelId } }));
  }

  private handleServerMessage(msg: any) {
    switch (msg.type) {
      case 'game.state':
      case 'game.session.started':
        if (!this.overlay.isPaused() && !this.quizOpen) this.updateFromState(msg.data);
        break;
      case 'game.action.rejected': this.showRejectMessage(msg.data?.error || 'ACTION REJECTED'); break;
      case 'game.over': this.upgradePanel.hide(); this.overlay.showMistakesSummaryWindow(false); break;
      case 'game.victory': this.upgradePanel.hide(); this.overlay.showMistakesSummaryWindow(true); break;
      case 'game.quiz.presented': this.quiz.showWindow(msg.data); break;
      case 'game.quiz.unavailable': this.quiz.clear(); this.showRejectMessage('NO QUIZZES REMAINING'); break;
      case 'game.quiz.result': this.quiz.handleResult(msg.data); break;
      case 'game.exited': this.overlay.completePendingExit(); break;
    }

    if (msg.type == 'game.state') {
      const waveIndex = msg.data?.wave || 0;
      if (this.currentWave !== waveIndex) {
        this.currentWave = waveIndex;
        this.bgm.updateForWave(waveIndex);
      }
    }
  }

  private setupShutdown() {
    this.events.once('shutdown', () => {
      if (this.ws) this.ws.onmessage = null;
      this.quiz.destroy();
      this.entities.destroy();
      this.overlay.destroy();
      this.upgradePanel.destroy();
    });
  }

  // ─── State sync ──────────────────────────────────────────────────────────────

  private updateFromState(state: any) {
    if (!state) return;
    if (state.session_id !== undefined) this.sessionId = state.session_id;
    if (state.essence !== undefined) this.currentEssence = state.essence;
    this.hud.update({ health: state.health, essence: state.essence, wave: state.wave });
    if (state.birds !== undefined) this.entities.syncTowers(state.birds);
    if (state.smogs !== undefined) this.entities.syncSmogs(state.smogs);
    if (state.projectiles !== undefined) this.entities.syncProjectiles(state.projectiles);
  }

  // ─── UI feedback ─────────────────────────────────────────────────────────────

  private showRejectMessage(errorText: string) {
    const pointer = this.input.activePointer;
    const warning = this.add.text(pointer.x, pointer.y - 20, errorText.toUpperCase(), {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '28px', color: '#ef4444', fontStyle: 'bold',
    }).setOrigin(0.5).setDepth(20);
    this.tweens.add({
      targets: warning, y: pointer.y - 100, alpha: 0,
      duration: 1600, ease: 'Quad.easeOut', onComplete: () => warning.destroy(),
    });
  }

  // ─── Utility ─────────────────────────────────────────────────────────────────

  private sendWs(type: string, data?: object) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data ? { type, data } : { type }));
    }
  }
}
