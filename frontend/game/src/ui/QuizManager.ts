import Phaser from 'phaser';

function isMobileDevice(): boolean {
  const ua = navigator.userAgent || navigator.vendor || (window as any).opera;
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(ua)
    || (navigator.maxTouchPoints > 2 && /Macintosh/.test(ua));
}

export class QuizHUDButton {
  public button: Phaser.GameObjects.Sprite | Phaser.GameObjects.Image;
  public labelBg: Phaser.GameObjects.NineSlice;
  public label: Phaser.GameObjects.Text;
  public cooldownText: Phaser.GameObjects.Text | null = null;
  public cooldownRemaining = 0;
  public isGameStarted = false;
  private baseScale = 1.0;

  constructor(
    private scene: Phaser.Scene,
    private config: {
      x: number;
      y: number;
      textureKey: string;
      hoverTextureKey?: string;
      labelText: string;
      size: number;
      isSprite?: boolean;
      labelYOffset?: number;
      flipX?: boolean;
      onClick: () => void;
    }
  ) {
    const { x, y, textureKey, hoverTextureKey, labelText, size, isSprite, labelYOffset, flipX, onClick } = config;

    // Create button
    if (isSprite) {
      this.button = this.scene.add.sprite(x, y, textureKey).setDepth(30);
    } else {
      this.button = this.scene.add.image(x, y, textureKey).setDepth(31);
    }
    this.button.setInteractive({ useHandCursor: true });
    if (flipX) {
      this.button.setFlipX(true);
    }

    this.baseScale = size / this.button.width;
    this.button.setScale(this.baseScale);

    // Create label background
    const labelY = y + (labelYOffset !== undefined ? labelYOffset : (size * 0.61));
    this.labelBg = this.scene.add.nineslice(x, labelY, 'box_white_outline_square', undefined, 210, 48, 16, 16, 16, 16)
      .setDepth(31).setAlpha(0.5);

    // Create label
    this.label = this.scene.add.text(x, labelY, labelText, {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '22px',
      color: '#64748b',
    }).setOrigin(0.5).setDepth(32);

    // Hover listeners
    const onOver = () => {
      if (!this.isGameStarted || this.cooldownRemaining > 0) return;
      if (hoverTextureKey) {
        this.button.setTexture(hoverTextureKey);
      }
      this.button.setScale(this.baseScale * 1.08);
      this.labelBg.setScale(1.08);
      this.label.setScale(1.08).setColor('#fef3c7');
    };

    const onOut = () => {
      const active = this.isGameStarted && this.cooldownRemaining === 0;
      if (hoverTextureKey) {
        this.button.setTexture(textureKey);
      }
      this.button.setScale(this.baseScale).setAlpha(active ? 1.0 : 0.5);
      this.labelBg.setScale(1.0).setAlpha(active ? 1.0 : 0.5);
      this.label.setScale(1.0).setColor(active ? '#ffffff' : '#64748b');
    };

    this.button.on('pointerover', onOver);
    this.button.on('pointerout', onOut);
    this.button.on('pointerdown', onClick);

    this.labelBg.setInteractive({ useHandCursor: true });
    this.labelBg.on('pointerover', onOver);
    this.labelBg.on('pointerout', onOut);
    this.labelBg.on('pointerdown', onClick);
  }

  setGameStarted(started: boolean) {
    this.isGameStarted = started;
    this.updateCooldown(this.cooldownRemaining);
  }

  private resetVisuals() {
    if (this.config.hoverTextureKey) {
      this.button.setTexture(this.config.textureKey);
    }
    this.button.setScale(this.baseScale);
    this.labelBg.setScale(1.0);
    this.label.setScale(1.0);
  }

  updateCooldown(seconds: number) {
    this.cooldownRemaining = Math.max(0, seconds);
    const active = this.isGameStarted && this.cooldownRemaining === 0;

    this.button.setAlpha(active ? 1.0 : 0.5);
    this.labelBg.setAlpha(active ? 1.0 : 0.5);
    this.label.setColor(active ? '#ffffff' : '#64748b');

    if (active) {
      this.button.setInteractive({ useHandCursor: true });
      this.labelBg.setInteractive({ useHandCursor: true });
      this.cooldownText?.setVisible(false);
    } else {
      this.resetVisuals();
      this.button.disableInteractive();
      this.labelBg.disableInteractive();

      if (this.isGameStarted && this.cooldownRemaining > 0) {
        if (!this.cooldownText) {
          this.cooldownText = this.scene.add.text(this.button.x, this.button.y, '', {
            fontFamily: '"Concert One", system-ui, sans-serif',
            fontSize: '48px',
            color: '#ffffff',
            fontStyle: 'bold',
          }).setOrigin(0.5).setDepth(34);
        }
        this.cooldownText.setText(String(this.cooldownRemaining)).setVisible(true);
      } else {
        this.cooldownText?.setVisible(false);
      }
    }
  }

  destroy() {
    this.button.destroy();
    this.labelBg.destroy();
    this.label.destroy();
    this.cooldownText?.destroy();
  }
}

export class QuizManager {
  private currentQuizId = '';
  private quizAnswered = false;
  private promptContainer: Phaser.GameObjects.Container | null = null;
  private messageHandler: ((e: MessageEvent) => void) | null = null;
  private hudBtn!: QuizHUDButton;

  constructor(
    private scene: Phaser.Scene,
    private ws: WebSocket | null,
  ) {}

  createHUD() {
    const width = this.scene.scale.width;
    const height = this.scene.scale.height;
    const rightX = width - 100;
    const centerY = height / 2;
    const birdY = centerY - 100;

    this.hudBtn = new QuizHUDButton(this.scene, {
      x: rightX,
      y: birdY,
      textureKey: 'bird_wisdom_1',
      hoverTextureKey: 'bird_wisdom_2',
      labelText: 'Bird of Wisdom',
      size: 200,
      isSprite: true,
      onClick: () => this.activate(),
    });
  }

  hidePrompt() {
    if (!this.promptContainer) return;
    const container = this.promptContainer;
    this.scene.tweens.add({
      targets: container,
      alpha: 0, scale: 0.8, duration: 250,
      onComplete: () => {
        container.destroy();
        if (this.promptContainer === container) this.promptContainer = null;
      },
    });
  }

  request(keepIframe = false) {
    if (!keepIframe) this.clear();
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'game.quiz.request' }));
    }
  }

  clear() {
    const overlay = document.getElementById('quiz-overlay');
    if (overlay) {
      overlay.remove();
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'game.resume' }));
      }
    }
    this.removeMessageHandler();
    if (this.hudBtn) {
      this.hudBtn.label.setColor(this.hudBtn.cooldownRemaining === 0 ? '#ffffff' : '#64748b');
    }
    this.quizAnswered = false;
  }

  showWindow(promptData: any) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'game.pause' }));
    }
    const existing = document.getElementById('quiz-iframe') as HTMLIFrameElement;
    if (existing?.contentWindow) {
      this.currentQuizId = promptData.quiz_id;
      this.quizAnswered = false;
      existing.contentWindow.postMessage({ type: 'quiz-presented', data: promptData }, '*');
      return;
    }

    this.clear();
    this.currentQuizId = promptData.quiz_id;

    const parent = document.getElementById('game-container') || document.body;

    const overlay = document.createElement('div');
    overlay.id = 'quiz-overlay';
    Object.assign(overlay.style, {
      position: 'absolute', top: '0', left: '0',
      width: '100%', height: '100%',
      backgroundColor: 'transparent', zIndex: '9999',
    });

    const iframe = document.createElement('iframe');
    iframe.id = 'quiz-iframe';
    Object.assign(iframe.style, {
      position: 'absolute', top: '0', left: '0',
      width: '100%', height: '100%',
      border: 'none', backgroundColor: 'transparent',
    });

    const params = new URLSearchParams({
      quiz_id: promptData.quiz_id,
      question: promptData.question_markdown,
      options: JSON.stringify(promptData.options_markdown || []),
      type: promptData.quiz_type === 'true_false' ? 'tf' : 'mcq',
    });
    if (isMobileDevice()) params.set('mobile', 'true');

    iframe.src = `${window.location.origin}/quiz-overlay?${params.toString()}`;
    overlay.appendChild(iframe);
    parent.appendChild(overlay);

    this.messageHandler = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      const packet = event.data;
      if (packet.type === 'quiz-submit') this.submitAnswer(packet.index);
      else if (packet.type === 'quiz-close') this.clear();
      else if (packet.type === 'quiz-next') this.request(true);
    };
    window.addEventListener('message', this.messageHandler);
  }

  submitAnswer(index: number) {
    if (this.quizAnswered) return;
    this.quizAnswered = true;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({
        type: 'game.quiz.answer',
        data: { quiz_id: this.currentQuizId, selected_index: index },
      }));
    }
  }

  handleResult(resultData: any) {
    const iframe = document.getElementById('quiz-iframe') as HTMLIFrameElement;
    iframe?.contentWindow?.postMessage({ type: 'quiz-result', data: resultData }, '*');
    if (resultData?.correct) this.hidePrompt();
  }

  destroy() {
    this.clear();
    this.hudBtn?.destroy();
  }

  setCooldownActive() {
    this.updateCooldown(30);
  }

  setGameStarted(started: boolean) {
    this.hudBtn?.setGameStarted(started);
    if (started && this.hudBtn?.cooldownRemaining === 0) {
      this.showPrompt();
    }
  }

  showPrompt() {
    if (this.promptContainer) return;

    const rightX = this.scene.scale.width - 130;
    const centerY = this.scene.scale.height / 2;

    const container = this.scene.add.container(rightX - 70, centerY - 250).setDepth(35).setScale(1.25).setAlpha(0);
    container.add([
      this.scene.add.image(0, 0, 'textbox_blank_side').setFlipX(true).setOrigin(0.5),
      this.scene.add.text(0, -10, 'Click on me to\nanswer a quiz and\nearn more sky essence!', {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '23px', color: '#000000ff', align: 'center',
      }).setOrigin(0.5),
    ]);
    this.promptContainer = container;

    this.scene.tweens.add({
      targets: container,
      alpha: 1, duration: 250,
    });

    this.scene.tweens.add({
      targets: container,
      y: { from: centerY - 250, to: centerY - 265 },
      duration: 1000, yoyo: true, repeat: -1, ease: 'Sine.easeInOut',
    });
  }

  private activate() {
    if (!this.hudBtn || !this.hudBtn.button.active) return;
    if (this.hudBtn.cooldownRemaining > 0) return;
    this.hidePrompt();
    this.request();
  }

  updateCooldown(seconds: number) {
    this.hudBtn?.updateCooldown(seconds);
    if (this.hudBtn && this.hudBtn.cooldownRemaining === 0 && this.hudBtn.isGameStarted) {
      this.showPrompt();
    }
  }

  private removeMessageHandler() {
    if (this.messageHandler) {
      window.removeEventListener('message', this.messageHandler);
      this.messageHandler = null;
    }
  }
}
