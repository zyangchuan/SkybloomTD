import Phaser from 'phaser';
import Tower from '../components/Tower';
import { QuizHUDButton } from './QuizManager';

type GridTarget = { x: number; y: number };

type AirstrikeState = {
  status?: string;
  charges?: number;
  cooldown_remaining_seconds?: number;
};

type AirstrikeConfig = {
  tileSize: number;
  offsetX: number;
  offsetY: number;
  gridWidth: number;
  gridHeight: number;
};

function isMobileDevice(): boolean {
  const ua = navigator.userAgent || navigator.vendor || (window as any).opera;
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(ua)
    || (navigator.maxTouchPoints > 2 && /Macintosh/.test(ua));
}

export class AirstrikeManager {
  private charges = 0;
  private status = 'empty';
  private gameStarted = false;
  private aiming = false;
  private awaitingPrepare = false;
  private impactSent = false;
  private acquireRequested = false;
  private currentQuizId = '';
  private currentQuizPrompt: any = null;
  private quizAnswered = false;
  private cooldownRemaining = 0;
  private targets: GridTarget[] = [];
  private markers: Phaser.GameObjects.Image[] = [];
  private cursorMarker: Phaser.GameObjects.Image | null = null;
  private hudBtn!: QuizHUDButton;
  private countBadge!: Phaser.GameObjects.Text;
  private countBadgeBg!: Phaser.GameObjects.Image;
  private messageHandler: ((event: MessageEvent) => void) | null = null;
  private escapeHandler: (() => void) | null = null;

  constructor(
    private scene: Phaser.Scene,
    private send: (type: string, data?: object) => void,
    private config: AirstrikeConfig,
  ) {}

  createHUD() {
    const x = this.scene.scale.width - 100;
    const y = this.scene.scale.height / 2 + 180;

    this.hudBtn = new QuizHUDButton(this.scene, {
      x,
      y,
      textureKey: 'consumable_airstrike_icon',
      labelText: 'Airstrike',
      size: 200,
      labelYOffset: 100,
      flipX: true,
      onClick: () => this.activate(),
    });

    this.countBadgeBg = this.scene.add.image(x - 82, y - 70, 'btn_bg_circle')
      .setScale(0.65)
      .setDepth(32);

    this.countBadge = this.scene.add.text(x - 82, y - 70, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '24px',
      color: '#ffffff',
    }).setOrigin(0.5).setDepth(33);

    this.updateHUD();
  }

  updateInventory(consumables: any) {
    const airstrike: AirstrikeState | undefined = consumables?.airstrike;
    if (!airstrike) return;
    this.charges = Math.max(0, airstrike.charges || 0);
    this.status = airstrike.status || (this.charges > 0 ? 'ready' : 'empty');
    this.cooldownRemaining = Math.max(0, airstrike.cooldown_remaining_seconds || 0);
    if (this.impactSent && this.cooldownRemaining > 0) {
      this.impactSent = false;
      this.awaitingPrepare = false;
    }
    this.updateHUD();
  }

  updateCooldown(seconds: number) {
    this.cooldownRemaining = Math.max(0, seconds);
    this.updateHUD();
  }

  setGameStarted(started: boolean) {
    this.gameStarted = started;
    this.hudBtn?.setGameStarted(started);
    if (!started) this.cancelAiming();
    this.updateHUD();
  }

  isQuizPending() {
    return this.acquireRequested || this.status === 'quiz_pending' || this.currentQuizId !== '';
  }

  isAiming() {
    return this.aiming;
  }

  showQuiz(promptData: any) {
    this.acquireRequested = false;
    this.currentQuizId = promptData.quiz_id;
    this.currentQuizPrompt = promptData;
    this.quizAnswered = false;
    this.status = 'quiz_pending';
    this.updateHUD();
    this.send('game.pause');

    const parent = document.getElementById('game-container') || document.body;
    let overlay = document.getElementById('airstrike-quiz-overlay');
    let iframe = document.getElementById('airstrike-quiz-iframe') as HTMLIFrameElement | null;
    if (iframe?.contentWindow) {
      iframe.contentWindow.postMessage({ type: 'quiz-presented', data: this.asConsumablePrompt(promptData) }, '*');
      return;
    }

    this.clearQuiz(false);
    overlay = document.createElement('div');
    overlay.id = 'airstrike-quiz-overlay';
    Object.assign(overlay.style, {
      position: 'absolute', inset: '0', width: '100%', height: '100%',
      backgroundColor: 'transparent', zIndex: '9999',
    });
    iframe = document.createElement('iframe');
    iframe.id = 'airstrike-quiz-iframe';
    Object.assign(iframe.style, {
      position: 'absolute', inset: '0', width: '100%', height: '100%',
      border: 'none', backgroundColor: 'transparent',
    });
    const params = new URLSearchParams({
      quiz_id: promptData.quiz_id,
      question: promptData.question_markdown,
      options: JSON.stringify(promptData.options_markdown || []),
      type: promptData.quiz_type === 'true_false' ? 'tf' : 'mcq',
      is_consumable: 'true',
      consumable_type: 'airstrike',
    });
    if (isMobileDevice()) params.set('mobile', 'true');
    iframe.src = `${window.location.origin}/quiz-overlay?${params.toString()}`;
    overlay.appendChild(iframe);
    parent.appendChild(overlay);

    this.messageHandler = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      if (event.data?.type === 'quiz-submit') this.submitQuizAnswer(event.data.index);
      else if (event.data?.type === 'quiz-close') this.clearQuiz();
    };
    window.addEventListener('message', this.messageHandler);
  }

  handleQuizResult(result: any) {
    this.acquireRequested = false;
    this.updateInventory(result?.consumables);
    if (result?.correct === false) this.updateCooldown(15);
    const iframe = document.getElementById('airstrike-quiz-iframe') as HTMLIFrameElement | null;
    iframe?.contentWindow?.postMessage({
      type: 'quiz-result',
      data: { ...result, is_consumable: true, consumable_type: 'airstrike' },
    }, '*');
    this.currentQuizId = '';
    this.currentQuizPrompt = null;
  }

  handleQuizUnavailable() {
    this.acquireRequested = false;
    this.currentQuizId = '';
    this.currentQuizPrompt = null;
    this.clearQuiz();
  }

  handleRejected() {
    if (!this.awaitingPrepare) return;
    this.awaitingPrepare = false;
    this.impactSent = false;
    this.charges++;
    this.cooldownRemaining = 0;
    this.cancelAiming();
    this.updateHUD();
  }

  destroy() {
    this.cancelAiming();
    this.clearQuiz(false);
    this.hudBtn?.destroy();
    this.countBadge?.destroy();
    this.countBadgeBg?.destroy();
  }

  private activate() {
    if (!this.gameStarted || this.awaitingPrepare || this.cooldownRemaining > 0) return;
    if (this.aiming) {
      this.cancelAiming();
      return;
    }
    if (this.charges <= 0) {
      if (this.status === 'quiz_pending' && this.currentQuizPrompt) {
        this.showQuiz(this.currentQuizPrompt);
      } else {
        this.acquireRequested = true;
        this.send('game.consumable.acquire', { item_type: 'airstrike' });
      }
      return;
    }
    this.startAiming();
  }

  private startAiming() {
    this.aiming = true;
    this.targets = [];
    this.scene.input.setDefaultCursor('none');
    this.cursorMarker = this.scene.add.image(0, 0, 'consumable_airstrike_aim_marker')
      .setDisplaySize(this.config.tileSize * 0.9, this.config.tileSize * 0.9)
      .setAlpha(0.9)
      .setDepth(45);
    this.scene.input.on('pointermove', this.moveCursor, this);
    this.scene.input.on('pointerdown', this.selectTarget, this);
    this.escapeHandler = () => this.cancelAiming();
    this.scene.input.keyboard?.on('keydown-ESC', this.escapeHandler);
    this.hudBtn.label.setText('Select 3 targets').setColor('#fef08a');
  }

  private moveCursor(pointer: Phaser.Input.Pointer) {
    this.cursorMarker?.setPosition(pointer.x, pointer.y);
  }

  private selectTarget(pointer: Phaser.Input.Pointer, objects: Phaser.GameObjects.GameObject[]) {
    if (!this.aiming || this.awaitingPrepare) return;
    const hasUI = objects.some(obj => !(obj instanceof Tower));
    if (hasUI) return;
    const target = this.pointerToGrid(pointer);
    if (!target || this.targets.some(existing => existing.x === target.x && existing.y === target.y)) return;
    this.targets.push(target);
    const center = this.gridToWorld(target);
    const marker = this.scene.add.image(center.x, center.y, 'consumable_airstrike_aim_marker')
      .setDisplaySize(this.config.tileSize * 0.9, this.config.tileSize * 0.9)
      .setDepth(44);
    this.scene.tweens.add({
      targets: marker,
      scaleX: marker.scaleX * 1.16,
      scaleY: marker.scaleY * 1.16,
      alpha: 0.55,
      duration: 500,
      yoyo: true,
      repeat: -1,
    });
    this.markers.push(marker);
    this.hudBtn.label.setText(`${3 - this.targets.length} targets left`);
    if (this.targets.length === 3) {
      this.awaitingPrepare = true;
      this.aiming = false;
      this.stopAimingInput();
      this.charges = Math.max(0, this.charges - 1);
      this.updateHUD();
      this.hudBtn.label.setText('Airstrike inbound').setColor('#fef08a');
      this.playDeployment([...this.targets], 1500);
    }
  }

  private pointerToGrid(pointer: Phaser.Input.Pointer): GridTarget | null {
    const x = Math.floor((pointer.x - this.config.offsetX) / this.config.tileSize);
    const y = Math.floor((pointer.y - this.config.offsetY) / this.config.tileSize);
    if (x < 0 || y < 0 || x >= this.config.gridWidth || y >= this.config.gridHeight) return null;
    return { x, y };
  }

  private gridToWorld(target: GridTarget) {
    return {
      x: this.config.offsetX + (target.x + 0.5) * this.config.tileSize,
      y: this.config.offsetY + (target.y + 0.5) * this.config.tileSize,
    };
  }

  private playDeployment(targets: GridTarget[], duration: number) {
    const sorted = [...targets].sort((a, b) => a.x - b.x);
    const planeY = Math.max(80, Math.min(...sorted.map(target => this.gridToWorld(target).y)) - this.config.tileSize * 1.5);
    const plane = this.scene.add.image(-100, planeY, 'consumable_airstrike_plane').setDepth(50).setScale(2.1).setAngle(-90);
    const startX = -100;
    const endX = this.scene.scale.width + 100;

    this.scene.tweens.add({
      targets: plane,
      x: endX,
      duration,
      ease: 'Linear',
      onComplete: () => {
        plane.destroy();
      },
    });

    let completedImpacts = 0;
    sorted.forEach(target => {
      const world = this.gridToWorld(target);
      const progress = Phaser.Math.Clamp((world.x - startX) / (endX - startX), 0.1, 0.85);
      this.scene.time.delayedCall(duration * progress, () => {
        this.dropBomb(world.x, planeY, world, () => {
          completedImpacts++;
          if (completedImpacts === sorted.length) {
            this.send('game.action.use_consumable', { item_type: 'airstrike', targets });
            this.impactSent = true;
            this.cooldownRemaining = 30;
            this.clearMarkers();
            this.updateHUD();
          }
        });
      });
    });
  }

  private dropBomb(spawnX: number, spawnY: number, target: { x: number; y: number }, onImpact: () => void) {
    const bomb = this.scene.add.image(spawnX, spawnY + 15, 'consumable_airstrike_bomb').setDepth(49).setScale(1.05).setAngle(-90);
    this.scene.tweens.add({
      targets: bomb,
      x: target.x,
      y: target.y,
      scale: 0.54,
      duration: 280,
      ease: 'Quad.easeIn',
      onComplete: () => {
        bomb.destroy();
        this.explode(target.x, target.y);
        onImpact();
      },
    });
  }

  private explode(x: number, y: number) {
    const blast = this.scene.add.circle(x, y, 30, 0xfb923c, 0.45).setDepth(48);
    const core = this.scene.add.circle(x, y, 18, 0xfef3c7, 0.6).setDepth(49);
    this.scene.cameras.main.shake(170, 0.007);
    this.scene.tweens.add({ targets: blast, radius: this.config.tileSize * 4, alpha: 0, duration: 350, onComplete: () => blast.destroy() });
    this.scene.tweens.add({ targets: core, radius: this.config.tileSize * 3, alpha: 0, duration: 220, onComplete: () => core.destroy() });

    if (!this.scene.anims.exists('airstrike_explode')) {
      this.scene.anims.create({
        key: 'airstrike_explode',
        frames: this.scene.anims.generateFrameNumbers('consumable_airstrike_explosion', { start: 0, end: 14 }),
        frameRate: 24,
        repeat: 0
      });
    }

    const scale = (this.config.tileSize * 2.5) / 317;
    const explosionSprite = this.scene.add.sprite(x, y, 'consumable_airstrike_explosion')
      .setDepth(51)
      .setScale(scale)
      .play('airstrike_explode');

    explosionSprite.once('animationcomplete', () => {
      explosionSprite.destroy();
    });
  }

  private submitQuizAnswer(index: number) {
    if (this.quizAnswered || !this.currentQuizId) return;
    this.quizAnswered = true;
    this.send('game.consumable.quiz.answer', {
      item_type: 'airstrike', quiz_id: this.currentQuizId, selected_index: index,
    });
  }

  private clearQuiz(resume = true) {
    document.getElementById('airstrike-quiz-overlay')?.remove();
    if (this.messageHandler) window.removeEventListener('message', this.messageHandler);
    this.messageHandler = null;
    this.quizAnswered = false;
    if (resume) this.send('game.resume');
  }

  private asConsumablePrompt(prompt: any) {
    return { ...prompt, is_consumable: true, consumable_type: 'airstrike' };
  }

  private cancelAiming(clearMarkers = true) {
    this.aiming = false;
    this.awaitingPrepare = false;
    this.stopAimingInput();
    if (clearMarkers) this.clearMarkers();
    this.targets = [];
    this.updateHUD();
  }

  private stopAimingInput() {
    this.scene.input.setDefaultCursor('default');
    this.scene.input.off('pointermove', this.moveCursor, this);
    this.scene.input.off('pointerdown', this.selectTarget, this);
    if (this.escapeHandler) this.scene.input.keyboard?.off('keydown-ESC', this.escapeHandler);
    this.escapeHandler = null;
    this.cursorMarker?.destroy();
    this.cursorMarker = null;
  }

  private clearMarkers() {
    this.markers.forEach(marker => marker.destroy());
    this.markers = [];
  }

  private updateHUD() {
    if (!this.hudBtn) return;
    this.countBadge.setText(String(this.charges));
    this.hudBtn.updateCooldown(this.cooldownRemaining);

    const enabled = this.gameStarted && !this.awaitingPrepare && this.cooldownRemaining === 0;
    const alphaVal = enabled ? 1.0 : 0.5;
    this.countBadge.setAlpha(alphaVal);
    this.countBadgeBg.setAlpha(alphaVal);

    if (this.aiming) return;
    this.hudBtn.label.setText('Airstrike');
  }
}
