import Phaser from 'phaser';

export default class BootScene extends Phaser.Scene {
  private loadingText!: Phaser.GameObjects.Text;
  private detailText!: Phaser.GameObjects.Text;
  private ws: WebSocket | null = null;
  private pollIntervalId: any = null;
  private subChapterId: string | null = null;
  private loadingTween!: Phaser.Tweens.Tween;
  private levelId: string | null = null; // Stored level ID to pass to GameScene

  constructor() {
    super('BootScene');
  }

  init() {
    const params = new URLSearchParams(window.location.search);
    this.subChapterId = params.get('sub_chapter_id');
    this.cameras.main.setBackgroundColor('#0f172a');
  }

  preload() {
    const { width, height } = this.scale;

    // Background gradient/graphics
    const graphics = this.add.graphics();
    graphics.fillStyle(0x0f172a, 1);
    graphics.fillRect(0, 0, width, height);

    // Preload all tile and object assets
    this.load.image('grass', '/game/assets/grass_floor.png');
    this.load.image('path_horiz', '/game/assets/path_straight_horizontal.png');
    this.load.image('path_vert', '/game/assets/path_straight_vertical.png');

    // Corner turn paths
    this.load.image('path_corner_ne', '/game/assets/path_turn_north_to_east.png');
    this.load.image('path_corner_se', '/game/assets/path_turn_south_to_east.png');
    this.load.image('path_corner_sw', '/game/assets/path_turn_south_to_west.png');
    this.load.image('path_corner_wn', '/game/assets/path_turn_west_to_north.png');

    // Object assets
    this.load.image('tree_01', '/game/assets/tree_01.png');
    this.load.image('tree_02', '/game/assets/tree_02.png');
    this.load.image('tree_03', '/game/assets/tree_03.png');

    this.load.image('tree_stump_01', '/game/assets/tree_stump_01.png');
    this.load.image('tree_stump_02', '/game/assets/tree_stump_02.png');

    this.load.image('bush_01', '/game/assets/bush_01.png');
    this.load.image('bush_02', '/game/assets/bush_02.png');
    this.load.image('bush_03', '/game/assets/bush_03.png');

    this.load.image('rock_01', '/game/assets/rock_01.png');
    this.load.image('rock_02', '/game/assets/rock_02.png');
    this.load.image('rock_03', '/game/assets/rock_03.png');
    this.load.image('rock_04', '/game/assets/rock_04.png');
    this.load.image('rock_05', '/game/assets/rock_05.png');

    // Preload HUD SVG icons
    this.load.svg('icon_heart', '/game/assets/gui/icons/Icon_Small_HeartFull.svg');
    this.load.svg('icon_essence', '/game/assets/gui/icons/Icon_Small_CoinDollar.svg');
    this.load.svg('icon_pause', '/game/assets/gui/icons/Icon_Small_WhiteOutline_Pause.svg');

    // Preload bird item assets
    this.load.svg('box_square', '/game/assets/gui/boxes_banners/Box_Square.svg', { width: 256, height: 256 });
    this.load.svg('box_orange_square', '/game/assets/gui/boxes_banners/Box_Orange_Square.svg', { width: 256, height: 256 });
    this.load.svg('btn_blue_round', '/game/assets/gui/buttons_text/ButtonText_Small_Blue_Round.svg', { width: 200, height: 80 });
    this.load.svg('btn_blank_round', '/game/assets/gui/buttons_text/ButtonText_Small_Blank_Round.svg', { width: 200, height: 80 });
    this.load.svg('btn_orange_round', '/game/assets/gui/buttons_text/ButtonText_Small_Orange_Round.svg', { width: 200, height: 80 });
    this.load.svg('textbox_blank_side', '/game/assets/gui/boxes_banners/TextBox_Blank_Side.svg', { width: 450, height: 160 });
    this.load.image('head_sparrow', '/game/assets/birds/sparrow_head.png');
    this.load.image('head_woodpecker', '/game/assets/birds/woodpecker_head.png');
    this.load.image('head_eagle', '/game/assets/birds/eagle_head.png');
    this.load.image('head_peacock', '/game/assets/birds/peacock_head.png');

    // Preload bird tower sprites
    this.load.image('tower_sparrow', '/game/assets/birds/sparrow_tower.png');
    this.load.image('tower_woodpecker', '/game/assets/birds/woodpecker_tower.png');
    this.load.image('tower_eagle', '/game/assets/birds/eagle_tower.png');
    this.load.image('tower_peacock', '/game/assets/birds/peacock_tower.png');

    // Preload enemies
    this.load.image('enemy_smog', '/game/assets/enemies/smog.png');
    this.load.image('enemy_junk', '/game/assets/enemies/junk.png');
    this.load.image('enemy_noise', '/game/assets/enemies/noise.png');

    // Preload bird projectiles
    this.load.image('projectile_sparrow', '/game/assets/birds/sparrow_projectile.png');
    this.load.image('projectile_woodpecker', '/game/assets/birds/woodpecker_projectile.png');
    this.load.image('projectile_eagle', '/game/assets/birds/eagle_projectile.png');
    this.load.image('projectile_peacock', '/game/assets/birds/peacock_projectile.png');
  }

  create() {
    this.sys.game.canvas.classList.remove('has-shadow');
    const { width, height } = this.scale;

    // Central card container using box_orange_square NineSlice
    const card = this.add.nineslice(width / 2, height / 2 + 10, 'box_orange_square', 0, 900, 500, 32, 32, 32, 32);
    card.setOrigin(0.5);

    this.loadingText = this.add.text(width / 2, height / 2 - 30, 'Preparing your level...', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '64px',
      fontStyle: 'bold',
      color: '#451a03',
    }).setOrigin(0.5);

    this.loadingTween = this.tweens.add({
      targets: this.loadingText,
      alpha: { from: 1, to: 0.5 },
      duration: 1200,
      yoyo: true,
      repeat: -1,
      ease: 'Sine.easeInOut'
    });

    this.detailText = this.add.text(width / 2, height / 2 + 50, 'Connecting to server...', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '32px',
      color: '#78350f',
    }).setOrigin(0.5);

    if (!this.subChapterId) {
      this.showError('No level specified in URL parameters.');
      return;
    }

    this.events.once('shutdown', this.shutdownScene, this);
    this.events.once('destroy', this.shutdownScene, this);

    this.connectWebSocket();
  }

  private connectWebSocket() {
    try {
      const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const wsUrl = `${protocol}://${window.location.host}/api/game-service/ws`;

      this.detailText.setText('Opening websocket connection...');
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.detailText.setText('Connected. Starting quiz generation...');
        this.startGameGeneration();
      };

      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          console.log('Received message from server:', message);
          this.handleWebSocketMessage(message);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        this.showError('WebSocket connection failed.');
      };
    } catch (err: any) {
      this.showError(`Connection setup failed: ${err.message}`);
    }
  }

  private startGameGeneration() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.showError('Websocket not open.');
      return;
    }

    const startMsg = {
      type: 'game.start',
      data: {
        sub_chapter_id: this.subChapterId
      }
    };

    this.ws.send(JSON.stringify(startMsg));
  }

  private handleWebSocketMessage(message: any) {
    switch (message.type) {
      case 'level_generation.started':
        const { generation_id, status_url, level_id } = message.data;

        if (level_id) {
          this.levelId = level_id; // Save retrieved level ID
          this.detailText.setText('Level ready! Loading game session...');
          this.loadGameSession(level_id);
        } else if (status_url) {
          this.detailText.setText('Generating map and study quizzes...');
          this.startPollingStatus(status_url);
        } else if (generation_id) {
          const fallbackStatusUrl = `/api/game-service/level-generation/${generation_id}/status`;
          this.detailText.setText('Generating map and study quizzes (fallback)...');
          this.startPollingStatus(fallbackStatusUrl);
        } else {
          this.showError('Invalid level generation message received.');
        }
        break;

      case 'game.initial_state':
        this.detailText.setText('Session loaded! Booting scene...');
        this.cleanup();

        if (this.ws) {
          this.ws.onopen = null;
          this.ws.onmessage = null;
          this.ws.onerror = null;
          this.ws.onclose = null;
        }

        const transferredWs = this.ws;
        this.ws = null; // Detach reference to prevent close during transition shutdown

        this.scene.start('GameScene', {
          initialState: message.data,
          ws: transferredWs,
          levelId: this.levelId
        });
        break;

      case 'error':
        this.showError(message.data?.error || 'An error occurred during level preparation.');
        break;

      default:
        console.warn('Unhandled message type:', message.type);
    }
  }

  private startPollingStatus(statusUrl: string) {
    if (this.pollIntervalId) {
      clearInterval(this.pollIntervalId);
    }

    this.pollIntervalId = setInterval(async () => {
      try {
        const response = await fetch(statusUrl);
        if (!response.ok) {
          throw new Error(`HTTP error ${response.status}`);
        }

        const data = await response.json();

        if (data.status === 'complete') {
          clearInterval(this.pollIntervalId);
          this.pollIntervalId = null;

          this.levelId = data.level_id; // Save retrieved level ID from poll response
          this.detailText.setText('Level completed! Launching...');
          this.loadGameSession(data.level_id);
        } else if (data.status === 'failed') {
          clearInterval(this.pollIntervalId);
          this.pollIntervalId = null;
          this.showError(data.error || 'Level generation failed on worker.');
        } else {
          if (data.map_status === 'running' || data.quiz_status === 'running') {
            this.detailText.setText('Indexing document content and building quizzes...');
          }
        }
      } catch (err: any) {
        console.error('Status poll failed:', err);
      }
    }, 1500);
  }

  private loadGameSession(levelId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.showError('Connection lost. Cannot load game session.');
      return;
    }

    const loadMsg = {
      type: 'game.load',
      data: {
        level_id: levelId
      }
    };

    this.ws.send(JSON.stringify(loadMsg));
  }

  private showError(message: string) {
    this.cleanup();

    if (this.loadingTween) {
      this.loadingTween.stop();
    }

    this.loadingText.setText('Preparation Failed')
      .setColor('#ef4444');

    this.detailText.setText(message).setColor('#f87171');

    const retryBtn = this.add.text(this.scale.width / 2, this.scale.height / 2 + 140, 'RETRY CONNECTION', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '32px',
      fontStyle: 'bold',
      color: '#10b981',
      backgroundColor: '#064e3b',
      padding: { x: 40, y: 20 }
    }).setOrigin(0.5).setInteractive({ useHandCursor: true });

    retryBtn.on('pointerdown', () => {
      this.scene.restart();
    });

    retryBtn.on('pointerover', () => retryBtn.setColor('#34d399'));
    retryBtn.on('pointerout', () => retryBtn.setColor('#10b981'));
  }

  private cleanup() {
    if (this.pollIntervalId) {
      clearInterval(this.pollIntervalId);
      this.pollIntervalId = null;
    }
  }

  private shutdownScene() {
    this.cleanup();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}
