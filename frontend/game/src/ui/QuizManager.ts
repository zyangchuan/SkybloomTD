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

    const quizBtn = this.scene.add.sprite(rightX, birdY, 'bird_wisdom_1')
      .setDepth(30).setScale(0.5).setInteractive({ useHandCursor: true });

    const labelBg = this.scene.add.nineslice(rightX, labelY, 'box_white_outline_square', undefined, 270, 66, 32, 32, 32, 32)
      .setDepth(31).setInteractive({ useHandCursor: true });

    const quizLabel = this.scene.add.text(rightX, labelY, 'Bird of Wisdom', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '30px', color: '#cbd5e1',
    }).setOrigin(0.5).setDepth(32);

    const onOver = () => {
      quizBtn.setTexture('bird_wisdom_2');
      labelBg.setScale(1.08);
      quizLabel.setScale(1.08).setColor('#fef3c7');
    };
    const onOut = () => {
      quizBtn.setTexture('bird_wisdom_1');
      labelBg.setScale(1.0);
      quizLabel.setScale(1.0).setColor('#cbd5e1');
    };
    const onDown = () => {
      this.hidePrompt();
      this.request();
    };

    quizBtn.on('pointerover', onOver);
    quizBtn.on('pointerout', onOut);
    quizBtn.on('pointerdown', onDown);

    labelBg.on('pointerover', onOver);
    labelBg.on('pointerout', onOut);
    labelBg.on('pointerdown', onDown);

    const container = this.scene.add.container(rightX - 70, centerY - 250).setDepth(35).setScale(1.25);
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
      y: { from: centerY - 250, to: centerY - 265 },
      duration: 1000, yoyo: true, repeat: -1, ease: 'Sine.easeInOut',
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
    document.getElementById('quiz-overlay')?.remove();
    this.removeMessageHandler();
    this.quizAnswered = false;
  }

  showWindow(promptData: any) {
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
  }

  private removeMessageHandler() {
    if (this.messageHandler) {
      window.removeEventListener('message', this.messageHandler);
      this.messageHandler = null;
    }
  }
}
