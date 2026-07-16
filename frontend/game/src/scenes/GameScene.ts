import Phaser from 'phaser';
import { MapRenderer } from '../map/MapRenderer';
import { GameHUD } from '../ui/GameHUD';
import { BirdTray } from '../ui/BirdTray';
import { QuizManager } from '../ui/QuizManager';
import { GameOverlay } from '../ui/GameOverlay';
import { DragController } from '../input/DragController';
import { EntitySync } from '../game/EntitySync';

export default class GameScene extends Phaser.Scene {
  private ws: WebSocket | null = null;
  private levelId = '';
  private sessionId = '';

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
  private startBtn: Phaser.GameObjects.Sprite | null = null;
  private startLabel: Phaser.GameObjects.Text | null = null;
  private startArrow: Phaser.GameObjects.Sprite | null = null;

  constructor() { super('GameScene'); }

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
    this.quiz.createHUD();

    this.drag = new DragController(
      this,
      (birdType, x, y) => this.sendWs('game.action.place_tower', { bird_type: birdType, x, y }),
      (sourceId, targetId) => this.sendWs('game.action.merge_tower', { source_bird_id: sourceId, target_bird_id: targetId }),
      (sourceBirdType, targetId) => this.sendWs('game.action.merge_tower', { source_bird_type: sourceBirdType, target_bird_id: targetId }),
      { tileSize: this.tileSize, offsetX: this.offsetX, offsetY: this.offsetY, gridWidth: this.gridWidth, gridHeight: this.gridHeight },
      this.enemyPath, this.obstacles, this.entities.towers,
    );
    this.drag.setup();

    new BirdTray(this, () => this.drag.isDragging());
    this.setupWebSocket(data.levelId);
    this.setupShutdown();
  }

  update(_time: number, delta: number) {
    if (this.overlay.isPaused()) return;
    this.entities.interpolate(delta);
  }

  private initGrid(mapData: any) {
    this.gridWidth  = mapData.width  || 18;
    this.gridHeight = mapData.height || 12;
    this.tileSize   = Math.floor(Math.min(this.scale.width / this.gridWidth, this.scale.height / this.gridHeight));
    this.offsetX    = (this.scale.width  - this.gridWidth  * this.tileSize) / 2;
    this.offsetY    = (this.scale.height - this.gridHeight * this.tileSize) / 2;
    this.enemyPath  = mapData.enemy_path || [];
    this.obstacles  = mapData.objects    || [];
  }

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
        if (!this.overlay.isPaused()) this.updateFromState(msg.data);
        break;
      case 'game.action.rejected':  this.showRejectMessage(msg.data?.error || 'ACTION REJECTED'); break;
      case 'game.over':             this.overlay.showMistakesSummaryWindow(false); break;
      case 'game.victory':          this.overlay.showMistakesSummaryWindow(true);  break;
      case 'game.quiz.presented':   this.quiz.showWindow(msg.data); break;
      case 'game.quiz.unavailable':
        this.quiz.clear();
        if (msg.data && msg.data.reason === 'quiz_cooldown') {
          this.quiz.setCooldownActive();
          this.showRejectMessage('QUIZ ON COOLDOWN');
        } else {
          this.showRejectMessage('NO QUIZZES REMAINING');
        }
        break;
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

  private updateFromState(state: any) {
    if (!state) return;
    if (state.session_id !== undefined) this.sessionId = state.session_id;
    this.hud.update({ health: state.health, essence: state.essence, wave: state.wave });
    if (state.birds       !== undefined) this.entities.syncTowers(state.birds);
    if (state.projectiles !== undefined) this.entities.syncProjectiles(state.projectiles);
    if (state.events      !== undefined) this.entities.processEvents(state.events);
    if (state.enemies     !== undefined) this.entities.syncEnemies(state.enemies);
    if (state.loop_started !== undefined) {
      this.quiz.setGameStarted(state.loop_started);
      if (state.loop_started) {
        this.destroyStartButton();
      } else {
        this.showStartButton();
      }
    }
    if (state.quiz_cooldown_remaining_seconds !== undefined) {
      this.quiz.updateCooldown(state.quiz_cooldown_remaining_seconds);
    }
  }

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

  private showStartButton() {
    if (this.startBtn) return;

    const leftX = 250;
    const centerY = this.scale.height / 2;
    const btnY = centerY - 30;

    this.startBtn = this.add.sprite(leftX, btnY, 'btn_large_blue_round')
      .setScale(0.8)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });

    this.startLabel = this.add.text(leftX, btnY - 2, 'Start Game', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '40px',
      color: '#ffffff',
    }).setOrigin(0.5).setDepth(31);

    this.startArrow = this.add.sprite(leftX + 250, btnY, 'icon_arrow')
      .setScale(1.8)
      .setDepth(30);

    this.startBtn.on('pointerover', () => {
      this.startBtn?.setScale(0.85);
      this.startLabel?.setScale(1.05).setColor('#fef3c7');
    });
    this.startBtn.on('pointerout', () => {
      this.startBtn?.setScale(0.8);
      this.startLabel?.setScale(1.0).setColor('#ffffff');
    });
    this.startBtn.on('pointerdown', () => {
      this.sendWs('game.session.run', {});
      this.startBtn?.disableInteractive();
    });

    this.tweens.add({
      targets: this.startArrow,
      x: leftX + 210,
      duration: 600,
      yoyo: true,
      repeat: -1,
      ease: 'Sine.easeInOut'
    });
  }

  private destroyStartButton() {
    if (this.startBtn) {
      this.startBtn.destroy();
      this.startBtn = null;
    }
    if (this.startLabel) {
      this.startLabel.destroy();
      this.startLabel = null;
    }
    if (this.startArrow) {
      this.startArrow.destroy();
      this.startArrow = null;
    }
  }

  private sendWs(type: string, data?: object) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data ? { type, data } : { type }));
    }
  }
}
