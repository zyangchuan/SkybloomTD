import Phaser from 'phaser';

class BootScene extends Phaser.Scene {
  private loadingText!: Phaser.GameObjects.Text;
  private detailText!: Phaser.GameObjects.Text;
  private ws: WebSocket | null = null;
  private pollIntervalId: any = null;
  private subChapterId: string | null = null;
  private loadingTween!: Phaser.Tweens.Tween;

  constructor() {
    super('BootScene');
  }

  init() {
    const params = new URLSearchParams(window.location.search);
    this.subChapterId = params.get('sub_chapter_id');
  }

  preload() {
    const { width, height } = this.scale;
    
    // Background gradient/graphics
    const graphics = this.add.graphics();
    graphics.fillGradientStyle(0x0a0e17, 0x0a0e17, 0x121824, 0x121824, 1);
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
  }

  create() {
    const { width, height } = this.scale;

    this.loadingText = this.add.text(width / 2, height / 2 - 20, 'Preparing your level...', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '32px',
      fontStyle: 'bold',
      color: '#38bdf8',
    }).setOrigin(0.5);

    this.loadingText.setShadow(0, 0, '#38bdf8', 10, true, true);

    this.loadingTween = this.tweens.add({
      targets: this.loadingText,
      alpha: { from: 1, to: 0.5 },
      duration: 1200,
      yoyo: true,
      repeat: -1,
      ease: 'Sine.easeInOut'
    });

    this.detailText = this.add.text(width / 2, height / 2 + 40, 'Connecting to server...', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '18px',
      color: '#64748b',
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
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws';
      const wsUrl = `${protocol}://${window.location.host}/api/game-service/ws`;
      
      this.detailText.setText('Opening websocket connection...');
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.detailText.setText('Connected. Starting generation...');
        this.startGameGeneration();
      };

      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          this.handleWebSocketMessage(message);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        this.showError('WebSocket connection failed.');
      };

      this.ws.onclose = (event) => {
        console.log('WebSocket closed:', event);
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
    console.log('Received WebSocket message:', message);

    switch (message.type) {
      case 'level_generation.started':
        const { generation_id, status_url, level_id } = message.data;
        
        if (level_id) {
          this.detailText.setText('Level ready! Loading game session...');
          this.loadGameSession(level_id);
        } else if (status_url) {
          this.detailText.setText('Generating map resources...');
          this.startPollingStatus(status_url);
        } else if (generation_id) {
          const fallbackStatusUrl = `/api/game-service/level-generation/${generation_id}/status`;
          this.detailText.setText('Generating map resources (fallback)...');
          this.startPollingStatus(fallbackStatusUrl);
        } else {
          this.showError('Invalid level generation message received.');
        }
        break;

      case 'game.initial_state':
        this.detailText.setText('Session loaded! Booting scene...');
        this.cleanup();
        this.scene.start('GameScene', { initialState: message.data, ws: this.ws });
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
        console.log('Polled generation status:', data);

        if (data.status === 'complete') {
          clearInterval(this.pollIntervalId);
          this.pollIntervalId = null;
          
          this.detailText.setText('Level completed! Launching...');
          this.loadGameSession(data.level_id);
        } else if (data.status === 'failed') {
          clearInterval(this.pollIntervalId);
          this.pollIntervalId = null;
          this.showError(data.error || 'Level generation failed on worker.');
        } else {
          if (data.map_status === 'running' || data.quiz_status === 'running') {
            this.detailText.setText('Indexing contents and building map paths...');
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
      .setColor('#ef4444')
      .setShadow(0, 0, '#ef4444', 10, true, true);

    this.detailText.setText(message).setColor('#f87171');

    const retryBtn = this.add.text(this.scale.width / 2, this.scale.height / 2 + 100, 'RETRY CONNECTION', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '18px',
      fontStyle: 'bold',
      color: '#10b981',
      backgroundColor: '#064e3b',
      padding: { x: 24, y: 12 }
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

// Game Scene that renders the map and shows real-time gameplay HUD updates
class GameScene extends Phaser.Scene {
  private ws: WebSocket | null = null;

  // HUD elements
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;

  constructor() {
    super('GameScene');
  }

  create(data: { initialState: any, ws: WebSocket }) {
    console.log('GameScene successfully loaded with state:', data.initialState);
    
    this.ws = data.ws;
    const mapData = data.initialState?.map;
    if (!mapData) {
      console.error('No map data available in initial state.');
      return;
    }

    const gridWidth = mapData.width || 18;
    const gridHeight = mapData.height || 12;
    const tileSize = Math.floor(Math.min(this.scale.width / gridWidth, this.scale.height / gridHeight));
    const offsetX = (this.scale.width - gridWidth * tileSize) / 2;
    const offsetY = (this.scale.height - gridHeight * tileSize) / 2;

    // 1. Render grass floor as the base layer for every map cell
    for (let y = 0; y < gridHeight; y++) {
      for (let x = 0; x < gridWidth; x++) {
        const posX = offsetX + x * tileSize + tileSize / 2;
        const posY = offsetY + y * tileSize + tileSize / 2;
        this.add.image(posX, posY, 'grass')
          .setDisplaySize(tileSize, tileSize);
      }
    }

    // 2. Render the paths
    const pathTiles = mapData.enemy_path || [];
    pathTiles.forEach((tile: any) => {
      const posX = offsetX + tile.x * tileSize + tileSize / 2;
      const posY = offsetY + tile.y * tileSize + tileSize / 2;
      const spriteKey = this.getPathSpriteKey(tile);
      
      this.add.image(posX, posY, spriteKey)
        .setDisplaySize(tileSize, tileSize);
    });

    // 3. Render the obstacles (depth sorted naturally based on Y position)
    const objects = mapData.objects || [];
    objects.sort((a: any, b: any) => a.y - b.y);

    objects.forEach((obj: any) => {
      const posX = offsetX + obj.x * tileSize + tileSize / 2;
      const posY = offsetY + obj.y * tileSize + tileSize / 2;
      const spriteKey = this.getObjectSpriteKey(obj);
      
      this.add.image(posX, posY, spriteKey)
        .setDisplaySize(tileSize + 4, tileSize + 4);
    });

    // 4. Render sleek, premium top-left glassmorphic HUD panel
    const hudBg = this.add.graphics();
    hudBg.fillStyle(0x0a0e17, 0.75); // Dark translucent theme matching background
    hudBg.fillRoundedRect(24, 24, 480, 68, 16);
    hudBg.lineStyle(2, 0x38bdf8, 0.25); // Glowing soft blue border
    hudBg.strokeRoundedRect(24, 24, 480, 68, 16);

    // Health Icon & Text
    this.add.image(48, 58, 'icon_heart').setDisplaySize(38, 38);
    this.healthText = this.add.text(94, 58, '100', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '28px',
      color: '#f87171',
    }).setOrigin(0, 0.5);

    // Essence Icon & Text
    this.add.image(204, 58, 'icon_essence').setDisplaySize(38, 38);
    this.essenceText = this.add.text(250, 58, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '28px',
      color: '#fbbf24',
    }).setOrigin(0, 0.5);

    // Wave Text (No Icon)
    this.waveText = this.add.text(364, 58, 'WAVE 0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '28px',
      color: '#38bdf8',
    }).setOrigin(0, 0.5);

    // 5. Connect Real-time WebSocket Listeners for continuous state updates
    if (this.ws) {
      // Clear previous onmessage listener
      this.ws.onmessage = null;

      // Listen to server game snapshots
      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          console.log('GameScene received message:', message.type);

          if (message.type === 'game.state' || message.type === 'game.session.started') {
            this.updateHUD(message.data);
          }
        } catch (err) {
          console.error('Failed to parse WebSocket message in GameScene:', err);
        }
      };

      // Send game.session.start handshake
      const levelId = data.initialState.level?.level_id || mapData.level_id;
      const startSessionMsg = {
        type: 'game.session.start',
        data: {
          level_id: levelId
        }
      };
      this.ws.send(JSON.stringify(startSessionMsg));
      console.log('Sent game.session.start message for level:', levelId);
    }

    // Clean up websocket listeners on scene shutdown
    this.events.once('shutdown', () => {
      if (this.ws) {
        this.ws.onmessage = null;
      }
    });
  }

  private updateHUD(gameState: any) {
    if (!gameState) return;

    if (gameState.health !== undefined) {
      this.healthText.setText(String(gameState.health));
    }
    if (gameState.essence !== undefined) {
      this.essenceText.setText(String(gameState.essence));
    }
    if (gameState.wave !== undefined) {
      this.waveText.setText(`WAVE ${gameState.wave}`);
    }
  }

  private getPathSpriteKey(tile: any): string {
    const kind = tile.kind;
    const from = tile.from;
    const to = tile.to;
    
    if (kind === 'straight') {
      return tile.axis === 'vertical' ? 'path_vert' : 'path_horiz';
    }
    
    if (kind === 'start') {
      return (to === 'north' || to === 'south') ? 'path_vert' : 'path_horiz';
    }
    
    if (kind === 'end') {
      return (from === 'north' || from === 'south') ? 'path_vert' : 'path_horiz';
    }
    
    if (kind === 'turn') {
      const dirs = new Set([from, to]);
      if (dirs.has('north') && dirs.has('east')) {
        return 'path_corner_ne';
      }
      if (dirs.has('south') && dirs.has('east')) {
        return 'path_corner_se';
      }
      if (dirs.has('south') && dirs.has('west')) {
        return 'path_corner_sw';
      }
      if (dirs.has('west') && dirs.has('north')) {
        return 'path_corner_wn';
      }
    }
    
    return 'path_horiz';
  }

  private getObjectSpriteKey(obj: any): string {
    const type = obj.type;
    const hash = obj.x * 7 + obj.y * 13;
    
    if (type === 'tree') {
      const index = (hash % 3) + 1;
      return `tree_0${index}`;
    }
    if (type === 'tree_stump') {
      const index = (hash % 2) + 1;
      return `tree_stump_0${index}`;
    }
    if (type === 'bush') {
      const index = (hash % 3) + 1;
      return `bush_0${index}`;
    }
    if (type === 'rock') {
      const index = (hash % 5) + 1;
      return `rock_0${index}`;
    }
    
    return 'tree_01';
  }
}

const config: Phaser.Types.Core.GameConfig = {
  type: Phaser.AUTO,
  width: 1440,
  height: 960,
  parent: 'game-container',
  backgroundColor: '#0a0e17',
  scale: {
    mode: Phaser.Scale.FIT,
    autoCenter: Phaser.Scale.CENTER_BOTH,
  },
  scene: [BootScene, GameScene],
};

new Phaser.Game(config);
