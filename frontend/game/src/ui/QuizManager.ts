import Phaser from 'phaser';

function isMobileDevice(): boolean {
  const ua = navigator.userAgent || navigator.vendor || (window as any).opera;
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(ua)
    || (navigator.maxTouchPoints > 2 && /Macintosh/.test(ua));
}

export class QuizManager {
  private currentQuizId = '';
  private quizAnswered = false;
  private promptContainer: Phaser.GameObjects.Container | null = null;
  private messageHandler: ((e: MessageEvent) => void) | null = null;

  private quizBtn!: Phaser.GameObjects.Sprite;
  private labelBg!: Phaser.GameObjects.NineSlice;
  private quizLabel!: Phaser.GameObjects.Text;
  private cooldownText: Phaser.GameObjects.Text | null = null;
  private cooldownRemaining = 0;
  private isGameStarted = false;

  constructor(
    private scene: Phaser.Scene,
    private ws: WebSocket | null,
  ) {}

  /** Creates the floating "answer a quiz to start" prompt and the QUIZ button. */
  createHUD() {
    const width = this.scene.scale.width;
    const height = this.scene.scale.height;
    const rightX = width - 130;
    const centerY = height / 2;
    const birdY = centerY - 30;
    const labelY = centerY + 115;

    this.quizBtn = this.scene.add.sprite(rightX, birdY, 'bird_wisdom_1')
      .setDepth(30).setScale(0.5).setAlpha(0.5);

    this.labelBg = this.scene.add.nineslice(rightX, labelY, 'box_white_outline_square', undefined, 270, 66, 32, 32, 32, 32)
      .setDepth(31).setAlpha(0.5);

    this.quizLabel = this.scene.add.text(rightX, labelY, 'Bird of Wisdom', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '30px', color: '#64748b',
    }).setOrigin(0.5).setDepth(32);

    const onOver = () => {
      if (this.cooldownRemaining > 0) return;
      this.quizBtn.setTexture('bird_wisdom_2');
      this.labelBg.setScale(1.08);
      this.quizLabel.setScale(1.08).setColor('#fef3c7');
    };
    const onOut = () => {
      if (this.cooldownRemaining > 0) return;
      this.quizBtn.setTexture('bird_wisdom_1');
      this.labelBg.setScale(1.0);
      this.quizLabel.setScale(1.0).setColor('#cbd5e1');
    };
    const onDown = () => {
      if (this.cooldownRemaining > 0) return;
      this.resetButtonVisuals();
      this.quizLabel.setColor('#cbd5e1');
      this.hidePrompt();
      this.request();
    };

    this.quizBtn.on('pointerover', onOver);
    this.quizBtn.on('pointerout', onOut);
    this.quizBtn.on('pointerdown', onDown);

    this.labelBg.on('pointerover', onOver);
    this.labelBg.on('pointerout', onOut);
    this.labelBg.on('pointerdown', onDown);
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
    this.resetButtonVisuals();
    this.quizLabel.setColor(this.isGameStarted ? '#cbd5e1' : '#64748b');
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
    this.cooldownText?.destroy();
    this.cooldownText = null;
  }

  setCooldownActive() {
    this.cooldownRemaining = 15;
    this.updateCooldown(15);
  }

  setGameStarted(started: boolean) {
    this.isGameStarted = started;
    this.updateCooldown(this.cooldownRemaining);
    if (started && this.cooldownRemaining === 0) {
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
      this.scene.add.text(0, -10, 'Click on me\nto answer a quiz\nand earn more essence!', {
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

  private resetButtonVisuals() {
    if (!this.quizBtn || !this.labelBg || !this.quizLabel) return;
    this.quizBtn.setTexture('bird_wisdom_1');
    this.labelBg.setScale(1.0);
    this.quizLabel.setScale(1.0);
  }

  updateCooldown(seconds: number) {
    this.cooldownRemaining = seconds;
    if (!this.quizBtn || !this.labelBg || !this.quizLabel) return;

    const rightX = this.scene.scale.width - 130;
    const centerY = this.scene.scale.height / 2;
    const birdY = centerY - 30;

    if (!this.isGameStarted) {
      this.resetButtonVisuals();
      this.quizBtn.disableInteractive();
      this.labelBg.disableInteractive();
      this.quizBtn.setAlpha(0.5);
      this.labelBg.setAlpha(0.5);
      this.quizLabel.setColor('#64748b');
      if (this.cooldownText) {
        this.cooldownText.setVisible(false);
      }
      return;
    }

    if (seconds > 0) {
      this.resetButtonVisuals();
      this.quizBtn.disableInteractive();
      this.labelBg.disableInteractive();
      this.quizBtn.setAlpha(0.5);
      this.labelBg.setAlpha(0.5);
      this.quizLabel.setColor('#64748b');

      if (!this.cooldownText) {
        this.cooldownText = this.scene.add.text(rightX, birdY, '', {
          fontFamily: '"Concert One", system-ui, sans-serif',
          fontSize: '44px', color: '#ffffff',
          fontStyle: 'bold'
        }).setOrigin(0.5).setDepth(34);
      }
      this.cooldownText.setText(String(seconds)).setVisible(true);
    } else {
      this.quizBtn.setInteractive({ useHandCursor: true });
      this.labelBg.setInteractive({ useHandCursor: true });
      this.quizBtn.setAlpha(1.0);
      this.labelBg.setAlpha(1.0);
      this.quizLabel.setColor('#cbd5e1');

      if (this.cooldownText) {
        this.cooldownText.setVisible(false);
      }

      // Show prompt if the game has started and is running
      if (this.isGameStarted) {
        this.showPrompt();
      }
    }
  }

  private removeMessageHandler() {
    if (this.messageHandler) {
      window.removeEventListener('message', this.messageHandler);
      this.messageHandler = null;
    }
  }
}
