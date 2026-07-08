import Phaser from 'phaser';

function isMobileDevice(): boolean {
  const ua = navigator.userAgent || navigator.vendor || (window as any).opera;
  return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(ua)
    || (navigator.maxTouchPoints > 2 && /Macintosh/.test(ua));
}

export class QuizManager {
  private currentQuizId = '';
  private quizAnswered = false;
  private quizOpened = false;
  private promptContainer: Phaser.GameObjects.Container | null = null;
  private messageHandler: ((e: MessageEvent) => void) | null = null;

  constructor(
    private scene: Phaser.Scene,
    private ws: WebSocket | null,
  ) {}

  /** Creates the floating "answer a quiz to start" prompt and the QUIZ button. */
  createHUD() {
    const quizBtn = this.scene.add.sprite(1800, 640, 'btn_orange_round')
      .setDepth(30).setScale(1.0).setInteractive({ useHandCursor: true });
    const quizLabel = this.scene.add.text(1800, 640, 'QUIZ', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '24px', color: '#ffffff',
    }).setOrigin(0.5).setDepth(31);

    quizBtn.on('pointerover', () => { quizBtn.setScale(1.08); quizLabel.setScale(1.08).setColor('#fef3c7'); });
    quizBtn.on('pointerout',  () => { quizBtn.setScale(1.0);  quizLabel.setScale(1.0).setColor('#ffffff'); });
    quizBtn.on('pointerdown', () => { this.hidePrompt(); this.request(); });

    const container = this.scene.add.container(1750, 500).setDepth(35);
    container.add([
      this.scene.add.image(0, 0, 'textbox_blank_side').setFlipX(true).setOrigin(0.5),
      this.scene.add.text(0, -10, 'Answer one quiz correctly\nto start the game!', {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '22px', color: '#000000ff', align: 'center',
      }).setOrigin(0.5),
    ]);
    this.promptContainer = container;

    this.scene.tweens.add({
      targets: container,
      y: { from: 500, to: 490 },
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
    if (this.quizOpened) {
      this.quizOpened = false;
      this.scene.sound.resumeAll();
      /*
        the animation and tweens resume
      */
      //this.scene.tweens.resumeAll();
      //this.scene.anims.resumeAll();
    }
  }

  showWindow(promptData: any) {
    const existing = document.getElementById('quiz-iframe') as HTMLIFrameElement;
    if (existing?.contentWindow) {
      this.currentQuizId = promptData.quiz_id;
      this.quizOpened = true;
      this.quizAnswered = false;
      existing.contentWindow.postMessage({ type: 'quiz-presented', data: promptData }, '*');
      return;
    }

    this.clear();
    this.currentQuizId = promptData.quiz_id;
    this.quizAnswered = false;
    if (!this.quizOpened) {
      this.quizOpened = true;
      this.scene.sound.pauseAll();
      /* 
        the animation and tweens pause
      */
     //this.scene.tweens.pauseAll();
     //this.scene.anims.pauseAll();
    }

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
      else if (packet.type === 'quiz-close') { this.clear(); this.scene.sound.resumeAll(); }
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
    this.scene.sound.play(resultData?.correct ? 'sfx_quiz_correct' : 'sfx_quiz_wrong', { volume: 0.5 });
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
