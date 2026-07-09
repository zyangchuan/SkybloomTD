import Phaser from 'phaser';
import { BgmManager } from '../audio/BgmManager';
import { VolumeSlider } from './VolumeSlider';

function isMobileDevice(): boolean {
  const ua = navigator.userAgent || navigator.vendor || (window as any).opera;
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(ua)
    || (navigator.maxTouchPoints > 2 && /Macintosh/.test(ua));
}

export class GameOverlay {
  private pauseWindowOpen = false;
  private pendingExitContinuation: (() => void) | null = null;
  private pendingExitTimeoutId: number | null = null;
  private summaryMessageHandler: ((e: MessageEvent) => void) | null = null;

  constructor(
    private scene: Phaser.Scene,
    private sendWs: (type: string, data?: object) => void,
    private getSessionId: () => string,
    private getLevelId: () => string,
    private clearQuiz: () => void,
    private bgmManager: BgmManager
  ) {}

  // Helper to pause 
  enterPaused() {
    this.pauseWindowOpen = true;
    this.scene.tweens.pauseAll();
    this.scene.anims.pauseAll();
    this.bgmManager.pause();
    this.sendWs('game.pause');
  }

  // Helper to resume
  exitPaused() {
    this.pauseWindowOpen = false;
    this.scene.tweens.resumeAll();
    this.scene.anims.resumeAll();
    this.bgmManager.resume();
    this.sendWs('game.resume');
  }

  isPaused() { return this.pauseWindowOpen; }

  // ─── Pause window ────────────────────────────────────────────────────────────

  showPauseWindow() {
    if (document.getElementById('quiz-overlay') || this.pauseWindowOpen) return;
    this.enterPaused();

    const backdrop = this.scene.add.graphics();
    backdrop.fillStyle(0x000000, 0.65).fillRect(-2000, -2000, 6000, 5000);
    backdrop.setInteractive(new Phaser.Geom.Rectangle(-2000, -2000, 6000, 5000), Phaser.Geom.Rectangle.Contains).setDepth(100);

    const dialog = this.scene.add.nineslice(940, 555, 'box_orange_square', undefined, 610, 900, 64, 64, 64, 64).setDepth(101);
    const title  = this.scene.add.text(940, 300 - 100, 'PAUSED', {
      fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '56px', color: '#451a03',
    }).setOrigin(0.5).setDepth(102);

    const resumeBtn   = this.scene.add.sprite(940, 400 - 100, 'btn_blue_round').setOrigin(0.5, 0.5).setScale(1.1).setDepth(102).setInteractive({ useHandCursor: true });
    const resumeLabel = this.scene.add.text(940, 400 - 100, 'RESUME', {
      fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '24px', color: '#ffffff',
    }).setOrigin(0.5).setDepth(103);
    resumeBtn.on('pointerover', () => { resumeBtn.setScale(1.18); resumeLabel.setScale(1.08).setColor('#fef3c7'); });
    resumeBtn.on('pointerout',  () => { resumeBtn.setScale(1.1);  resumeLabel.setScale(1.0).setColor('#ffffff'); });
    resumeBtn.on('pointerdown', () => {
      this.exitPaused();
      [backdrop, dialog, title, resumeBtn, resumeLabel, exitBtn, exitLabel, music, volumeSlider].forEach(o => o.destroy());
    });

   const exitBtn   = this.scene.add.sprite(940, 600 - 100, 'btn_blank_round').setScale(1.1).setDepth(102).setInteractive({ useHandCursor: true });
   const exitLabel = this.scene.add.text(940, 600 - 100, 'EXIT GAME', {
      fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '24px', color: '#000000',
    }).setOrigin(0.5).setDepth(103);
    exitBtn.on('pointerover', () => { exitBtn.setScale(1.18); exitLabel.setScale(1.08); });
    exitBtn.on('pointerout',  () => { exitBtn.setScale(1.1);  exitLabel.setScale(1.0); });
    exitBtn.on('pointerdown', () => this.exitGameThen(() => this.navigateToDashboard()));


    const music = this.scene.add.text(940, 700 - 100, 'MUSIC', {
      fontFamily: '"Concert One", system-ui, sans-serif', fontSize: '56px', color: '#000000',
    }).setOrigin(0.5).setDepth(103);
    
    const volumeSlider = new VolumeSlider(this.scene, 1000, 700, (volume) => {
      this.bgmManager.setVolume(volume);
  });


  }

  // ─── Mistakes summary ────────────────────────────────────────────────────────

  showMistakesSummaryWindow(isVictory: boolean) {
    this.clearQuiz();
    document.getElementById('mistakes-overlay')?.remove();

    const parent = document.getElementById('game-container') || document.body;
    const overlay = document.createElement('div');
    overlay.id = 'mistakes-overlay';
    Object.assign(overlay.style, {
      position: 'absolute', top: '0', left: '0',
      width: '100%', height: '100%',
      backgroundColor: 'rgba(2, 6, 23, 0.75)', zIndex: '9999',
    });

    const iframe = document.createElement('iframe');
    iframe.id = 'mistakes-iframe';
    Object.assign(iframe.style, {
      position: 'absolute', top: '0', left: '0',
      width: '100%', height: '100%', border: 'none', backgroundColor: 'transparent',
    });

    const params = new URLSearchParams({
      level_id: this.getLevelId(),
      session_id: this.getSessionId(),
      victory: isVictory ? 'true' : 'false',
    });
    if (isMobileDevice()) params.set('mobile', 'true');

    iframe.src = `${window.location.origin}/mistakes-summary?${params.toString()}`;
    overlay.appendChild(iframe);
    parent.appendChild(overlay);

    this.summaryMessageHandler = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      if (event.data.type === 'game-replay') this.exitGameThen(() => window.location.reload());
      else if (event.data.type === 'game-exit') this.exitGameThen(() => this.navigateToDashboard());
    };
    window.addEventListener('message', this.summaryMessageHandler);
  }

  // ─── Exit flow ───────────────────────────────────────────────────────────────

  exitGameThen(continuation: () => void) {
    if (this.pendingExitContinuation) return;
    this.pendingExitContinuation = continuation;
    if (this.pendingExitTimeoutId !== null) window.clearTimeout(this.pendingExitTimeoutId);
    this.pendingExitTimeoutId = window.setTimeout(() => this.completePendingExit(), 2000);
    try {
      this.sendWs('game.exit', { session_id: this.getSessionId() });
    } catch (err) {
      console.error('Failed to send game.exit:', err);
      this.completePendingExit();
    }
  }

  completePendingExit() {
    const continuation = this.pendingExitContinuation;
    if (!continuation) return;
    this.pendingExitContinuation = null;
    if (this.pendingExitTimeoutId !== null) {
      window.clearTimeout(this.pendingExitTimeoutId);
      this.pendingExitTimeoutId = null;
    }
    continuation();
  }

  destroy() {
    if (this.summaryMessageHandler) {
      window.removeEventListener('message', this.summaryMessageHandler);
      this.summaryMessageHandler = null;
    }
    if (this.pendingExitTimeoutId !== null) {
      window.clearTimeout(this.pendingExitTimeoutId);
      this.pendingExitTimeoutId = null;
    }
  }

  private navigateToDashboard() {
    const p = new URLSearchParams(window.location.search);
    const doc = p.get('document_id'), ch = p.get('chapter_id');
    window.location.href = (doc && ch) ? `/dashboard/games/${doc}/chapters/${ch}` : '/dashboard';
  }
}
