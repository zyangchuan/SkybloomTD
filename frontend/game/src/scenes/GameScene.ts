import Phaser from 'phaser';
import { MapRenderer } from '../map/MapRenderer';
import { GameHUD } from '../ui/GameHUD';
import { BirdTray } from '../ui/BirdTray';
import { QuizManager } from '../ui/QuizManager';
import { GameOverlay } from '../ui/GameOverlay';
import { DragController } from '../input/DragController';
import { EntitySync } from '../game/EntitySync';
import { BgmManager } from '../audio/BgmManager';

export default class GameScene extends Phaser.Scene {
  private ws: WebSocket | null = null;
  private levelId = '';
  private sessionId = '';
  private currentWave = -1;

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
  private drag!: DragController;
  private entities!: EntitySync;
  private audio!: BgmManager;

  constructor() { super('GameScene'); }

  // ─── Lifecycle ───────────────────────────────────────────────────────────────

  create(data: { initialState: any, ws: WebSocket, levelId: string }) {
    this.ws = data.ws;
    this.levelId = data.levelId;
    const mapData = data.initialState?.map;
    if (!mapData) { console.error('No map data in initial state.'); return; }

    this.initGrid(mapData);
    new MapRenderer(this, this.tileSize, this.offsetX, this.offsetY, this.gridWidth, this.gridHeight).render(mapData);

    this.entities = new EntitySync(this, this.tileSize, this.offsetX, this.offsetY);
    this.overlay  = new GameOverlay(this, (t, d) => this.sendWs(t, d), () => this.sessionId, () => this.levelId, () => this.quiz.clear());
    this.hud      = new GameHUD(this, () => this.overlay.showPauseWindow());
    this.quiz     = new QuizManager(this, this.ws);
    this.audio    = new BgmManager(this);
    this.quiz.createHUD();

    this.sound.pauseOnBlur = false;

    this.drag = new DragController(
      this,
      (birdType, x, y) => this.sendWs('game.action.place_tower', { bird_type: birdType, x, y }),
      { tileSize: this.tileSize, offsetX: this.offsetX, offsetY: this.offsetY, gridWidth: this.gridWidth, gridHeight: this.gridHeight },
      this.enemyPath, this.obstacles, this.entities.towers,
    );
    this.drag.setup();

    new BirdTray(this, () => this.drag.isDragging());
    this.setupWebSocket(data.levelId);
    this.setupShutdown();
  }

  update() {
    if (this.overlay.isPaused()) return;
    this.entities.interpolate();
  }

  // ─── Grid ────────────────────────────────────────────────────────────────────

  private initGrid(mapData: any) {
    this.gridWidth  = mapData.width  || 18;
    this.gridHeight = mapData.height || 12;
    this.tileSize   = Math.floor(Math.min(this.scale.width / this.gridWidth, this.scale.height / this.gridHeight));
    this.offsetX    = (this.scale.width  - this.gridWidth  * this.tileSize) / 2;
    this.offsetY    = (this.scale.height - this.gridHeight * this.tileSize) / 2;
    this.enemyPath  = mapData.enemy_path || [];
    this.obstacles  = mapData.objects    || [];
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
        const waveIndex: number = msg.data?.wave || 0;
        if (waveIndex != this.currentWave) {
          this.audio.updateForWave(waveIndex);
          this.currentWave = waveIndex;
        }
      case 'game.session.started':
        if (!this.overlay.isPaused()) {
           this.updateFromState(msg.data);
        }
        break;
      case 'game.action.rejected':  this.showRejectMessage(msg.data?.error || 'ACTION REJECTED'); break;
      case 'game.over':             this.overlay.showMistakesSummaryWindow(false); break;
      case 'game.victory':          this.overlay.showMistakesSummaryWindow(true);  break;
      case 'game.quiz.presented':   this.quiz.showWindow(msg.data);  break;
      case 'game.quiz.unavailable': this.quiz.clear(); this.showRejectMessage('NO QUIZZES REMAINING'); break;
      case 'game.quiz.result':      this.quiz.handleResult(msg.data); break;
      case 'game.exited':           this.overlay.completePendingExit(); break;
    }
  }

  private setupShutdown() {
    this.events.once('shutdown', () => {
      if (this.ws) this.ws.onmessage = null;
      this.quiz.destroy();
      this.entities.destroy();
      this.overlay.destroy();
    });
  }

  // ─── State sync ──────────────────────────────────────────────────────────────

  private updateFromState(state: any) {
    if (!state) return;
    if (state.session_id !== undefined) this.sessionId = state.session_id;
    this.hud.update({ health: state.health, essence: state.essence, wave: state.wave });
    if (state.birds       !== undefined) this.entities.syncTowers(state.birds);
    if (state.smogs       !== undefined) this.entities.syncSmogs(state.smogs);
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