import Phaser from 'phaser';
import Tower from '../components/Tower';

export default class GameScene extends Phaser.Scene {
  private ws: WebSocket | null = null;

  // HUD elements
  private healthText!: Phaser.GameObjects.Text;
  private essenceText!: Phaser.GameObjects.Text;
  private waveText!: Phaser.GameObjects.Text;

  // Map state trackers
  private towers: Map<string, Tower> = new Map();
  private smogs: Map<string, {
    sprite: Phaser.GameObjects.Sprite,
    healthBar: Phaser.GameObjects.Graphics,
    targetX: number,
    targetY: number,
    health: number,
    maxHealth: number
  }> = new Map();
  private smogMaxHealth: Map<string, number> = new Map();
  private projectiles: Map<string, {
    sprite: Phaser.GameObjects.Sprite,
    targetX: number,
    targetY: number,
    targetRotation: number
  }> = new Map();
  public activeSmogsList: Array<{ id: string, x: number, y: number, pathIndex: number }> = [];
  private gridWidth: number = 18;
  private gridHeight: number = 12;
  private tileSize: number = 0;
  private offsetX: number = 0;
  private offsetY: number = 0;
  private enemyPath: any[] = [];
  private obstacles: any[] = [];

  private sessionId: string = '';

  // Drag and placement elements
  private activeDragSprite: Phaser.GameObjects.Sprite | null = null;
  private activeDragBirdType: string | null = null;
  private gridHighlightGraphics: Phaser.GameObjects.Graphics | null = null;
  private dragRangeGraphics: Phaser.GameObjects.Graphics | null = null;
  private closestCellHighlight: Phaser.GameObjects.Graphics | null = null;
  private pulseTween: Phaser.Tweens.Tween | null = null;
  private dragCancelLeftBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelLeftText: Phaser.GameObjects.Text | null = null;
  private dragCancelRightBox: Phaser.GameObjects.Graphics | null = null;
  private dragCancelRightText: Phaser.GameObjects.Text | null = null;
  private currentQuizId: string = '';
  private quizAnswered: boolean = false;
  private quizPromptContainer: Phaser.GameObjects.Container | null = null;
  private pauseWindowOpen: boolean = false;
  private messageHandler: ((e: MessageEvent) => void) | null = null;
  private levelId: string = '';

  private birdStats: Record<string, { damage: number, range: number, fireRate: string, attack: string, cost: number, color: string }> = {
    sparrow: { damage: 10, range: 3.5, fireRate: '1.0/s', attack: 'SINGLE', cost: 50, color: '#38bdf8' },
    woodpecker: { damage: 6, range: 3.5, fireRate: '2.0/s', attack: 'SINGLE', cost: 65, color: '#fb7185' },
    eagle: { damage: 30, range: 6.0, fireRate: '0.4/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
    peacock: { damage: 7, range: 3.5, fireRate: '1.0/s', attack: 'SPLASH', cost: 90, color: '#c084fc' }
  };

  constructor() {
    super('GameScene');
  }

  create(data: { initialState: any, ws: WebSocket, levelId: string }) {
    this.ws = data.ws;
    this.levelId = data.levelId;
    const mapData = data.initialState?.map;
    if (!mapData) {
      console.error('No map data available in initial state.');
      return;
    }

    // Save grid bounds and scale parameters
    this.gridWidth = mapData.width || 18;
    this.gridHeight = mapData.height || 12;
    this.tileSize = Math.floor(Math.min(this.scale.width / this.gridWidth, this.scale.height / this.gridHeight));
    this.offsetX = (this.scale.width - this.gridWidth * this.tileSize) / 2;
    this.offsetY = (this.scale.height - this.gridHeight * this.tileSize) / 2;
    this.enemyPath = mapData.enemy_path || [];
    this.obstacles = mapData.objects || [];

    // Initialize overlay graphics layers
    this.gridHighlightGraphics = this.add.graphics().setDepth(5);
    this.closestCellHighlight = this.add.graphics().setDepth(6);

    // 1. Render grass floor as the base layer for every map cell
    for (let y = 0; y < this.gridHeight; y++) {
      for (let x = 0; x < this.gridWidth; x++) {
        const posX = this.offsetX + x * this.tileSize + this.tileSize / 2;
        const posY = this.offsetY + y * this.tileSize + this.tileSize / 2;
        this.add.image(posX, posY, 'grass')
          .setDisplaySize(this.tileSize, this.tileSize);
      }
    }

    // 2. Render the paths
    this.enemyPath.forEach((tile: any) => {
      const posX = this.offsetX + tile.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + tile.y * this.tileSize + this.tileSize / 2;
      const spriteKey = this.getPathSpriteKey(tile);

      this.add.image(posX, posY, spriteKey)
        .setDisplaySize(this.tileSize, this.tileSize);
    });

    // 3. Render the obstacles (depth sorted naturally based on Y position)
    this.obstacles.sort((a: any, b: any) => a.y - b.y);
    this.obstacles.forEach((obj: any) => {
      const posX = this.offsetX + obj.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + obj.y * this.tileSize + this.tileSize / 2;
      const spriteKey = this.getObjectSpriteKey(obj);

      const obstacleImg = this.add.image(posX, posY, spriteKey);
      
      // Determine a reasonable target dimension based on obstacle type to keep size natural
      let targetHeight = this.tileSize;
      let targetWidth = this.tileSize;
      let useHeightAsBase = true; // whether to match targetHeight and scale width, or match targetWidth and scale height

      if (obj.type === 'tree') {
        // Trees should be nice and tall! Make the height about 1.85x of tileSize
        targetHeight = this.tileSize * 1.85;
        useHeightAsBase = true;
      } else if (obj.type === 'bush') {
        // Bushes should fit snug within the tile, slightly smaller (e.g. 0.85x of tileSize)
        targetWidth = this.tileSize * 0.85;
        useHeightAsBase = false;
      } else if (obj.type === 'rock') {
        // Rocks should look sturdy and robust, making rock_04 and rock_05 smaller!
        if (spriteKey === 'rock_04' || spriteKey === 'rock_05') {
          targetWidth = this.tileSize * 0.38;
        } else {
          targetWidth = this.tileSize * 0.85;
        }
        useHeightAsBase = false;
      } else if (obj.type === 'tree_stump') {
        // Stumps are flat and low (e.g. 0.8x of tileSize)
        targetWidth = this.tileSize * 0.8;
        useHeightAsBase = false;
      } else {
        // Default fallback to fill the grid cell elegantly
        targetHeight = this.tileSize;
        useHeightAsBase = true;
      }

      // Calculate scale to maintain aspect ratio perfectly
      if (useHeightAsBase) {
        const scale = targetHeight / obstacleImg.height;
        obstacleImg.setScale(scale);
      } else {
        const scale = targetWidth / obstacleImg.width;
        obstacleImg.setScale(scale);
      }

      obstacleImg.setDepth(1);
    });

    // 4. Render sleek, premium top-left HUD panel using box_orange_square as a NineSlice container
    this.add.nineslice(360, 69, 'box_orange_square', undefined, 700, 90, 32, 32, 32, 32).setDepth(30);

    // Health Icon & Text
    this.add.image(62, 69, 'icon_heart').setDisplaySize(50, 50).setDepth(30);
    this.healthText = this.add.text(124, 69, '100', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#7c0000ff',
    }).setOrigin(0, 0.5).setDepth(30);

    // Essence Icon & Text
    this.add.image(274, 69, 'icon_essence').setDisplaySize(50, 50).setDepth(30);
    this.essenceText = this.add.text(336, 69, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#a35700ff',
    }).setOrigin(0, 0.5).setDepth(30);

    // Wave Text (No Icon)
    this.waveText = this.add.text(484, 69, 'WAVE 0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#451a03',
    }).setOrigin(0, 0.5).setDepth(30);

    // Beautiful Pause HUD Button at top right
    const pauseBtnBg = this.add.nineslice(1840, 69, 'box_orange_square', undefined, 100, 90, 32, 32, 32, 32)
      .setDepth(30)
      .setInteractive({ useHandCursor: true });
    
    this.add.image(1840, 69, 'icon_pause')
      .setDisplaySize(50, 50)
      .setDepth(31);

    pauseBtnBg.on('pointerdown', () => {
      this.showPauseWindow();
    });

    // Sleek premium Quizz button on the right side of the screen
    const quizHUDBtn = this.add.sprite(1800, 640, 'btn_orange_round')
      .setDepth(30)
      .setScale(1.0)
      .setInteractive({ useHandCursor: true });
    const quizHUDLabel = this.add.text(1800, 640, 'QUIZ', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '24px',
      color: '#ffffff'
    }).setOrigin(0.5).setDepth(31);

    quizHUDBtn.on('pointerover', () => {
      quizHUDBtn.setScale(1.08);
      quizHUDLabel.setScale(1.08).setColor('#fef3c7');
    });
    quizHUDBtn.on('pointerout', () => {
      quizHUDBtn.setScale(1.0);
      quizHUDLabel.setScale(1.0).setColor('#ffffff');
    });

    quizHUDBtn.on('pointerdown', () => {
      this.hideQuizPrompt();
      this.requestQuiz();
    });

    // Mirror TextBox_Blank_Side horizontally (points left by default, flipX=true points right directly at QUIZ button)
    const promptContainer = this.add.container(1750, 500).setDepth(35);
    const promptBg = this.add.image(0, 0, 'textbox_blank_side')
      .setFlipX(true)
      .setOrigin(0.5);
    const promptText = this.add.text(0, -10, 'Answer one quiz correctly\nto start the game!', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '22px',
      color: '#000000ff',
      align: 'center'
    }).setOrigin(0.5);

    promptContainer.add([promptBg, promptText]);
    this.quizPromptContainer = promptContainer;

    // Smooth vertical floating animation above the quiz button
    this.tweens.add({
      targets: promptContainer,
      y: { from: 500, to: 490 },
      duration: 1000,
      yoyo: true,
      repeat: -1,
      ease: 'Sine.easeInOut'
    });

    // 5. Selectable Birds Tray Panel at Bottom Center using box_orange_square as NineSlice outer container
    this.add.nineslice(960, 1152, 'box_orange_square', undefined, 760, 170, 32, 32, 32, 32).setDepth(30);

    const birds = ['sparrow', 'woodpecker', 'eagle', 'peacock'];
    const startX = 637;
    const boxY = 1152;
    const boxSize = 136;
    const headSize = 98;

    birds.forEach((bird, index) => {
      const boxX = startX + index * 170 + boxSize / 2;

      // 1. Create Tooltip Container
      const tooltipContainer = this.add.container(boxX, boxY - 230).setDepth(35);
      tooltipContainer.setVisible(false);

      // Background plate using Box_Square NineSlice
      const tooltipBg = this.add.nineslice(0, 0, 'box_square', undefined, 300, 220, 32, 32, 32, 32);
      tooltipContainer.add(tooltipBg);

      // Title/Header
      const statsInfo = this.birdStats[bird];
      const title = this.add.text(0, -82, bird.toUpperCase(), {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '32px',
        color: statsInfo.color,
      }).setOrigin(0.5);
      tooltipContainer.add(title);

      // Stats rows
      const rows = [
        { label: 'DAMAGE', value: String(statsInfo.damage), color: '#f87171' },
        { label: 'RANGE', value: String(statsInfo.range), color: '#60a5fa' },
        { label: 'FIRE RATE', value: statsInfo.fireRate, color: '#34d399' },
        { label: 'ATTACK', value: statsInfo.attack, color: '#e9d5ff' },
        { label: 'COST', value: String(statsInfo.cost), color: '#fbbf24' }
      ];

      rows.forEach((row, rIndex) => {
        const rowY = -42 + rIndex * 28;

        // Key Label (left-aligned)
        const lbl = this.add.text(-120, rowY, row.label, {
          fontFamily: '"Concert One", system-ui, sans-serif',
          fontSize: '20px',
          color: '#94a3b8'
        }).setOrigin(0, 0.5);

        // Value Label (right-aligned)
        const val = this.add.text(120, rowY, row.value, {
          fontFamily: '"Concert One", system-ui, sans-serif',
          fontSize: '20px',
          color: row.color
        }).setOrigin(1, 0.5);

        tooltipContainer.add(lbl);
        tooltipContainer.add(val);
      });

      // 2. Container Box_Square as a NineSlice container
      const box = this.add.nineslice(boxX, boxY, 'box_square', undefined, boxSize, boxSize, 32, 32, 32, 32)
        .setInteractive({ useHandCursor: true, draggable: true })
        .setData('birdType', bird)
        .setDepth(30);

      // Bird Head representation
      const head = this.add.image(boxX, boxY - 14, `head_${bird}`)
        .setDisplaySize(headSize, headSize)
        .setDepth(31);

      // Label inside the bottom area of the container
      const label = this.add.text(boxX, boxY + 44, bird.toUpperCase(), {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '19px',
        color: '#94a3b8',
      }).setOrigin(0.5).setDepth(31);

      // Interactive hover dynamics (only expand when not dragging)
      box.on('pointerover', () => {
        if (this.activeDragBirdType) return;
        box.setSize(boxSize + 12, boxSize + 12);
        head.setDisplaySize(headSize + 8, headSize + 8).setY(boxY - 18);
        label.setColor('#ffffff').setY(boxY + 50);
        tooltipContainer.setVisible(true);
      });
      box.on('pointerout', () => {
        box.setSize(boxSize, boxSize);
        head.setDisplaySize(headSize, headSize).setY(boxY - 14);
        label.setColor('#94a3b8').setY(boxY + 44);
        tooltipContainer.setVisible(false);
      });
    });

    // 6. Connect Real-time Drag-and-Drop Placement Listeners
    this.input.on('dragstart', (pointer: Phaser.Input.Pointer, gameObject: Phaser.GameObjects.GameObject) => {
      const birdType = gameObject.getData('birdType');
      if (!birdType) return;

      this.activeDragBirdType = birdType;

      // Initialize drag range graphics layer
      this.dragRangeGraphics = this.add.graphics().setDepth(39);

      // Spawn temporary visual placement preview
      this.activeDragSprite = this.add.sprite(pointer.x, pointer.y, `tower_${birdType}`)
        .setAlpha(0.8)
        .setDepth(40);

      const dragScale = this.tileSize / this.activeDragSprite.width * 1.5;
      this.activeDragSprite.setScale(dragScale);

      // Spawn red translucent cancel zones on both sides of the birds bar
      this.dragCancelLeftBox = this.add.graphics().setDepth(30);
      this.dragCancelLeftBox.fillStyle(0xef4444, 0.6);
      this.dragCancelLeftBox.fillRoundedRect(280 - 540/2, 1152 - 170/2, 540, 170, 24);
      this.dragCancelLeftBox.setAlpha(0.75);

      this.dragCancelLeftText = this.add.text(280, 1152, 'RELEASE TO CANCEL', {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '26px',
        color: '#ffffff'
      }).setOrigin(0.5).setDepth(31);

      this.dragCancelRightBox = this.add.graphics().setDepth(30);
      this.dragCancelRightBox.fillStyle(0xef4444, 0.6);
      this.dragCancelRightBox.fillRoundedRect(1640 - 540/2, 1152 - 170/2, 540, 170, 24);
      this.dragCancelRightBox.setAlpha(0.75);

      this.dragCancelRightText = this.add.text(1640, 1152, 'RELEASE TO CANCEL', {
        fontFamily: '"Concert One", system-ui, sans-serif',
        fontSize: '26px',
        color: '#ffffff'
      }).setOrigin(0.5).setDepth(31);

      // Draw glowing overlays on all valid placement patches
      this.drawGrassHighlights();

      // Add white pulsing highlight tween
      this.gridHighlightGraphics?.setAlpha(0.2);
      this.pulseTween = this.tweens.add({
        targets: this.gridHighlightGraphics,
        alpha: { from: 0.2, to: 0.7 },
        duration: 700,
        yoyo: true,
        repeat: -1,
        ease: 'Sine.easeInOut'
      });
    });

    this.input.on('drag', (pointer: Phaser.Input.Pointer, _gameObject: Phaser.GameObjects.GameObject) => {
      if (!this.activeDragSprite) return;

      // Update dragging preview location to pointer coordinates
      this.activeDragSprite.setPosition(pointer.x, pointer.y);

      // Render drag range radius circle (light blue translucent with dark blue border)
      if (this.dragRangeGraphics && this.activeDragBirdType) {
        this.dragRangeGraphics.clear();
        const stats = this.birdStats[this.activeDragBirdType];
        if (stats) {
          const radius = stats.range * this.tileSize;
          
          // Light blue fill: 0x93c5fd at 0.3 opacity
          this.dragRangeGraphics.fillStyle(0x93c5fd, 0.3);
          this.dragRangeGraphics.fillCircle(pointer.x, pointer.y, radius);
          
          // Blue border: 0x2563eb at 0.8 opacity, 3px line width
          this.dragRangeGraphics.lineStyle(3, 0x2563eb, 0.8);
          this.dragRangeGraphics.strokeCircle(pointer.x, pointer.y, radius);
        }
      }

      // Dynamic highlights for cancel zones
      const hoverLeftCancel = pointer.y > 1050 && pointer.x < 580;
      const hoverRightCancel = pointer.y > 1050 && pointer.x > 1340;

      if (this.dragCancelLeftBox && this.dragCancelLeftText) {
        if (hoverLeftCancel) {
          this.dragCancelLeftBox.setAlpha(0.9);
          this.dragCancelLeftText.setScale(1.15).setColor('#fecaca');
        } else {
          this.dragCancelLeftBox.setAlpha(0.75);
          this.dragCancelLeftText.setScale(1.0).setColor('#ffffff');
        }
      }

      if (this.dragCancelRightBox && this.dragCancelRightText) {
        if (hoverRightCancel) {
          this.dragCancelRightBox.setAlpha(0.9);
          this.dragCancelRightText.setScale(1.15).setColor('#fecaca');
        } else {
          this.dragCancelRightBox.setAlpha(0.75);
          this.dragCancelRightText.setScale(1.0).setColor('#ffffff');
        }
      }

      // Calculate hovered grid indices
      const gridX = Math.floor((pointer.x - this.offsetX) / this.tileSize);
      const gridY = Math.floor((pointer.y - this.offsetY) / this.tileSize);

      this.closestCellHighlight?.clear();

      // Cancel preview if pointer is hovering over the bottom birds bar
      const inBirdsBar = pointer.y > 1050;

      if (!inBirdsBar && this.isValidGrassTile(gridX, gridY)) {
        // High visibility valid cell highlight
        const posX = this.offsetX + gridX * this.tileSize;
        const posY = this.offsetY + gridY * this.tileSize;
        this.closestCellHighlight?.fillStyle(0x34d399, 0.45);
        this.closestCellHighlight?.lineStyle(3, 0x10b981, 1);
        this.closestCellHighlight?.fillRect(posX, posY, this.tileSize, this.tileSize);
        this.closestCellHighlight?.strokeRect(posX, posY, this.tileSize, this.tileSize);
      } else {
        // Warning invalid cell highlight
        const posX = this.offsetX + gridX * this.tileSize;
        const posY = this.offsetY + gridY * this.tileSize;
        if (!inBirdsBar && gridX >= 0 && gridX < this.gridWidth && gridY >= 0 && gridY < this.gridHeight) {
          this.closestCellHighlight?.fillStyle(0xf87171, 0.35);
          this.closestCellHighlight?.lineStyle(3, 0xef4444, 1);
          this.closestCellHighlight?.fillRect(posX, posY, this.tileSize, this.tileSize);
          this.closestCellHighlight?.strokeRect(posX, posY, this.tileSize, this.tileSize);
        }
      }
    });

    this.input.on('dragend', (pointer: Phaser.Input.Pointer) => {
      if (this.dragRangeGraphics) {
        this.dragRangeGraphics.destroy();
        this.dragRangeGraphics = null;
      }
      if (!this.activeDragSprite) return;

      const gridX = Math.floor((pointer.x - this.offsetX) / this.tileSize);
      const gridY = Math.floor((pointer.y - this.offsetY) / this.tileSize);

      const inBirdsBar = pointer.y > 1050;

      // Dispatched placing actions (only if not dropped inside the bottom birds bar)
      if (!inBirdsBar && this.isValidGrassTile(gridX, gridY) && this.activeDragBirdType) {
        const placeMsg = {
          type: 'game.action.place_tower',
          data: {
            bird_type: this.activeDragBirdType,
            x: gridX,
            y: gridY
          }
        };
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(JSON.stringify(placeMsg));
        }
      }

      // Cleanup drag parameters
      this.activeDragSprite.destroy();
      this.activeDragSprite = null;
      this.activeDragBirdType = null;

      // Clean up cancel zones
      if (this.dragCancelLeftBox) {
        this.dragCancelLeftBox.destroy();
        this.dragCancelLeftBox = null;
      }
      if (this.dragCancelLeftText) {
        this.dragCancelLeftText.destroy();
        this.dragCancelLeftText = null;
      }
      if (this.dragCancelRightBox) {
        this.dragCancelRightBox.destroy();
        this.dragCancelRightBox = null;
      }
      if (this.dragCancelRightText) {
        this.dragCancelRightText.destroy();
        this.dragCancelRightText = null;
      }

      // Stop the pulsing tween
      if (this.pulseTween) {
        this.pulseTween.stop();
        this.pulseTween = null;
      }

      this.gridHighlightGraphics?.clear();
      this.gridHighlightGraphics?.setAlpha(1.0);
      this.closestCellHighlight?.clear();
    });

    // 7. Connect Real-time WebSocket Listeners for continuous state updates
    if (this.ws) {
      // Clear previous onmessage listener
      this.ws.onmessage = null;

      // Listen to server game snapshots
      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          console.log('Received message from server:', message);

          if (message.type === 'game.state' || message.type === 'game.session.started') {
            if (!this.pauseWindowOpen) this.updateHUD(message.data);
          } else if (message.type === 'game.action.rejected') {
            this.showRejectMessage(message.data?.error || 'ACTION REJECTED');
          } else if (message.type === 'game.over') {
            this.showMistakesSummaryWindow(false);
          } else if (message.type === 'game.victory') {
            this.showMistakesSummaryWindow(true);
          } else if (message.type === 'game.quiz.presented') {
            this.showQuizWindow(message.data);
          } else if (message.type === 'game.quiz.unavailable') {
            this.clearQuiz();
            this.showRejectMessage('NO QUIZZES REMAINING');
          } else if (message.type === 'game.quiz.result') {
            this.handleQuizResult(message.data);
          }
        } catch (err) {
          console.error('Failed to parse WebSocket message in GameScene:', err);
        }
      };

      // Send game.session.start handshake
      const levelId = data.levelId;
      const startSessionMsg = {
        type: 'game.session.start',
        data: {
          level_id: levelId
        }
      };
      this.ws.send(JSON.stringify(startSessionMsg));
    }

    // Clean up websocket listeners on scene shutdown
    this.events.once('shutdown', () => {
      if (this.ws) {
        this.ws.onmessage = null;
      }
      this.towers.forEach((tower) => tower.destroy());
      this.towers.clear();

      this.smogs.forEach((smog) => {
        smog.sprite.destroy();
        smog.healthBar.destroy();
      });
      this.smogs.clear();
      this.smogMaxHealth.clear();

      this.projectiles.forEach((proj) => {
        proj.sprite.destroy();
      });
      this.projectiles.clear();
    });
  }

  /**
   * Phaser scene update loop: runs on every frame to interpolate moving objects smoothly
   */
  update() {
    if (this.pauseWindowOpen) return;

    // 1. Update towers
    this.towers.forEach((tower) => {
      tower.update();
    });

    // 2. Interpolate moving smogs smoothly (lerp)
    this.smogs.forEach((smog) => {
      const { sprite, healthBar, targetX, targetY, health, maxHealth } = smog;

      // Standard linear interpolation towards authoritative target
      sprite.x = Phaser.Math.Linear(sprite.x, targetX, 0.18);
      sprite.y = Phaser.Math.Linear(sprite.y, targetY, 0.18);

      // Redraw health bar centered exactly above the interpolated sprite position
      healthBar.clear();
      const pct = Math.max(0, Math.min(1, health / maxHealth));

      const barWidth = 60;
      const barHeight = 8;
      const barX = sprite.x - barWidth / 2;
      const barY = sprite.y - this.tileSize / 2 - 12;

      // Draw health bar background shadow
      healthBar.fillStyle(0x1e293b, 1.0);
      healthBar.fillRoundedRect(barX, barY, barWidth, barHeight, 3);

      if (pct > 0) {
        let healthColor = 0x10b981; // emerald-500
        if (pct < 0.3) {
          healthColor = 0xef4444; // red-500
        } else if (pct < 0.6) {
          healthColor = 0xf59e0b; // amber-500
        }
        healthBar.fillStyle(healthColor, 1.0);
        healthBar.fillRoundedRect(barX + 1, barY + 1, (barWidth - 2) * pct, barHeight - 2, 2);
      }
    });

    // 3. Interpolate moving projectiles smoothly
    this.projectiles.forEach((proj) => {
      const { sprite, targetX, targetY, targetRotation } = proj;

      // Higher lerp factor for projectiles to feel crisp and fast
      sprite.x = Phaser.Math.Linear(sprite.x, targetX, 0.28);
      sprite.y = Phaser.Math.Linear(sprite.y, targetY, 0.28);

      // Interpolate rotation angle to prevent sharp snaps
      sprite.rotation = Phaser.Math.Angle.RotateTo(sprite.rotation, targetRotation, 0.25);
    });
  }

  /**
   * Renders the soft grid overlays indicating valid placement grass patches
   */
  private drawGrassHighlights() {
    this.gridHighlightGraphics?.clear();
    this.gridHighlightGraphics?.fillStyle(0xffffff, 0.3);
    this.gridHighlightGraphics?.lineStyle(2, 0xffffff, 1.0);

    for (let y = 0; y < this.gridHeight; y++) {
      for (let x = 0; x < this.gridWidth; x++) {
        if (this.isValidGrassTile(x, y)) {
          const posX = this.offsetX + x * this.tileSize;
          const posY = this.offsetY + y * this.tileSize;

          this.gridHighlightGraphics?.fillRect(posX, posY, this.tileSize, this.tileSize);
          this.gridHighlightGraphics?.strokeRect(posX, posY, this.tileSize, this.tileSize);
        }
      }
    }
  }

  /**
   * Validates if target tile location is plain grass and unoccupied
   */
  private isValidGrassTile(x: number, y: number): boolean {
    if (x < 0 || x >= this.gridWidth || y < 0 || y >= this.gridHeight) return false;

    // Reject enemy path cells
    const isOnPath = this.enemyPath.some((tile: any) => tile.x === x && tile.y === y);
    if (isOnPath) return false;

    // Reject obstacle cells
    const isObstacle = this.obstacles.some((obj: any) => obj.x === x && obj.y === y);
    if (isObstacle) return false;

    // Reject already occupied cells
    for (const tower of this.towers.values()) {
      if (tower.gridX === x && tower.gridY === y) {
        return false;
      }
    }

    return true;
  }

  /**
   * Pop dynamic floaty warning animation on server rejects (e.g. Insufficient Essence)
   */
  private showRejectMessage(errorText: string) {
    const pointer = this.input.activePointer;

    const warning = this.add.text(pointer.x, pointer.y - 20, errorText.toUpperCase(), {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '28px',
      color: '#ef4444',
      fontStyle: 'bold'
    }).setOrigin(0.5).setDepth(20);

    this.tweens.add({
      targets: warning,
      y: pointer.y - 100,
      alpha: 0,
      duration: 1600,
      ease: 'Quad.easeOut',
      onComplete: () => {
        warning.destroy();
      }
    });
  }

  /**
   * Authoritatively synchronizes bird towers from game server snapshot
   */
  private syncTowers(birdsList: any[]) {
    const activeIds = new Set<string>();

    birdsList.forEach((birdData: any) => {
      const { id, type, position, stats } = birdData;
      activeIds.add(id);

      const gridX = position.x;
      const gridY = position.y;

      if (!this.towers.has(id)) {
        const posX = this.offsetX + gridX * this.tileSize + this.tileSize / 2;
        const posY = this.offsetY + gridY * this.tileSize + this.tileSize / 2;

        const tower = new Tower(this, posX, posY, id, type, gridX, gridY);
        if (stats && stats.range) {
          tower.range = stats.range;
        }

        // Retain original aspect ratio perfectly by scaling proportionally to fit the tile size!
        const towerScale = this.tileSize / tower.width * 1.3;
        tower.setScale(towerScale);
        tower.setDepth(4); // Keep towers layered appropriately

        this.towers.set(id, tower);
      } else {
        const tower = this.towers.get(id)!;
        if (stats && stats.range) {
          tower.range = stats.range;
        }
      }
    });

    // Tear down decommissioned towers
    for (const [id, tower] of this.towers.entries()) {
      if (!activeIds.has(id)) {
        tower.destroy();
        this.towers.delete(id);
      }
    }
  }

  private syncSmogs(smogsList: any[]) {
    this.activeSmogsList = smogsList.map(smog => ({
      id: smog.id,
      x: smog.position.x,
      y: smog.position.y,
      pathIndex: smog.path_index || 0
    }));

    const activeIds = new Set<string>();

    smogsList.forEach((smogData: any) => {
      const { id, health, position } = smogData;
      activeIds.add(id);

      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;

      // Track max health statically the first time we see this smog
      if (!this.smogMaxHealth.has(id)) {
        this.smogMaxHealth.set(id, health);
      }
      const maxHealth = this.smogMaxHealth.get(id) || health || 1;

      if (!this.smogs.has(id)) {
        // Spawn smog sprite using the enemy_smog asset
        const sprite = this.add.sprite(posX, posY, 'enemy_smog');
        sprite.setDisplaySize(this.tileSize * 1.4, this.tileSize * 1.4);
        sprite.setDepth(15);

        // Spawn a companion graphics container for the health bar
        const healthBar = this.add.graphics();
        healthBar.setDepth(16);

        this.smogs.set(id, {
          sprite,
          healthBar,
          targetX: posX,
          targetY: posY,
          health,
          maxHealth
        });
      } else {
        const entry = this.smogs.get(id)!;
        entry.targetX = posX;
        entry.targetY = posY;
        entry.health = health;
        entry.maxHealth = maxHealth;
      }
    });

    // Clean up dead/completed smogs
    for (const [id, entry] of this.smogs.entries()) {
      if (!activeIds.has(id)) {
        entry.sprite.destroy();
        entry.healthBar.destroy();
        this.smogs.delete(id);
        this.smogMaxHealth.delete(id);
      }
    }
  }

  private syncProjectiles(projectilesList: any[]) {
    const activeIds = new Set<string>();

    projectilesList.forEach((projData: any) => {
      const { id, damage, position, direction } = projData;
      activeIds.add(id);

      const posX = this.offsetX + position.x * this.tileSize + this.tileSize / 2;
      const posY = this.offsetY + position.y * this.tileSize + this.tileSize / 2;

      // Determine projectile texture key from unique bird damage signatures
      let birdType = 'sparrow';
      if (damage === 6) {
        birdType = 'woodpecker';
      } else if (damage === 30) {
        birdType = 'eagle';
      } else if (damage === 7) {
        birdType = 'peacock';
      }
      const textureKey = `projectile_${birdType}`;

      let targetRotation = 0;
      if (direction && (direction.x !== 0 || direction.y !== 0)) {
        targetRotation = Math.atan2(direction.y, direction.x) + Math.PI / 2;
      }

      if (!this.projectiles.has(id)) {
        const sprite = this.add.sprite(posX, posY, textureKey);
        // Scale proportionally to preserve original aspect ratio
        const scale = this.tileSize / sprite.width * 0.5;
        sprite.setScale(scale);
        sprite.setDepth(20);
        sprite.setRotation(targetRotation);

        this.projectiles.set(id, {
          sprite,
          targetX: posX,
          targetY: posY,
          targetRotation
        });
      } else {
        const entry = this.projectiles.get(id)!;
        entry.targetX = posX;
        entry.targetY = posY;
        entry.targetRotation = targetRotation;
      }
    });

    // Clean up expired projectiles
    for (const [id, entry] of this.projectiles.entries()) {
      if (!activeIds.has(id)) {
        entry.sprite.destroy();
        this.projectiles.delete(id);
      }
    }
  }

  private updateHUD(gameState: any) {
    if (!gameState) return;

    if (gameState.session_id !== undefined) {
      this.sessionId = gameState.session_id;
    }

    if (gameState.health !== undefined) {
      this.healthText.setText(String(gameState.health));
    }
    if (gameState.essence !== undefined) {
      this.essenceText.setText(String(gameState.essence));
    }
    if (gameState.wave !== undefined) {
      this.waveText.setText(`WAVE ${gameState.wave}`);
    }
    if (gameState.birds !== undefined) {
      this.syncTowers(gameState.birds);
    }
    if (gameState.smogs !== undefined) {
      this.syncSmogs(gameState.smogs);
    }
    if (gameState.projectiles !== undefined) {
      this.syncProjectiles(gameState.projectiles);
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

  private showMistakesSummaryWindow(isVictory: boolean) {
    this.clearQuiz();

    const parent = document.getElementById('game-container') || document.body;

    // Create the overlay backdrop div
    const overlay = document.createElement('div');
    overlay.id = 'mistakes-overlay';
    overlay.style.position = 'absolute';
    overlay.style.top = '0';
    overlay.style.left = '0';
    overlay.style.width = '100%';
    overlay.style.height = '100%';
    overlay.style.backgroundColor = 'rgba(2, 6, 23, 0.75)'; // Slate-950 at 0.75 alpha backdrop
    overlay.style.zIndex = '9999';

    // Create the iframe pointing to the Next.js mistakes summary page
    const iframe = document.createElement('iframe');
    iframe.id = 'mistakes-iframe';
    iframe.style.position = 'absolute';
    iframe.style.top = '0';
    iframe.style.left = '0';
    iframe.style.width = '100%';
    iframe.style.height = '100%';
    iframe.style.border = 'none';
    iframe.style.backgroundColor = 'transparent';

    // Build query parameters
    const params = new URLSearchParams();
    params.set('level_id', this.levelId);
    params.set('session_id', this.sessionId);
    params.set('victory', isVictory ? 'true' : 'false');

    iframe.src = `${window.location.origin}/mistakes-summary?${params.toString()}`;
    overlay.appendChild(iframe);
    parent.appendChild(overlay);

    // Register a secure postMessage listener for child interactions (replay/exit)
    this.messageHandler = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;

      const packet = event.data;
      if (packet.type === 'game-replay') {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(JSON.stringify({
            type: 'game.exit',
            data: { session_id: this.sessionId }
          }));
        }
        setTimeout(() => {
          window.location.reload();
        }, 150);
      } else if (packet.type === 'game-exit') {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(JSON.stringify({
            type: 'game.exit',
            data: { session_id: this.sessionId }
          }));
        }
        setTimeout(() => {
          const searchParams = new URLSearchParams(window.location.search);
          const documentId = searchParams.get('document_id');
          const chapterId = searchParams.get('chapter_id');
          if (documentId && chapterId) {
            window.location.href = `/dashboard/games/${documentId}/chapters/${chapterId}`;
          } else {
            window.location.href = '/dashboard';
          }
        }, 150);
      }
    };
    window.addEventListener('message', this.messageHandler);
  }

  private hideQuizPrompt() {
    if (this.quizPromptContainer) {
      const container = this.quizPromptContainer;
      this.tweens.add({
        targets: container,
        alpha: 0,
        scale: 0.8,
        duration: 250,
        onComplete: () => {
          container.destroy();
          if (this.quizPromptContainer === container) {
            this.quizPromptContainer = null;
          }
        }
      });
    }
  }

  private requestQuiz(keepIframe: boolean = false) {
    if (!keepIframe) {
      this.clearQuiz();
    }
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'game.quiz.request' }));
    }
  }

  private clearQuiz() {
    const overlay = document.getElementById('quiz-overlay');
    if (overlay) {
      overlay.remove();
    }
    const mistakesOverlay = document.getElementById('mistakes-overlay');
    if (mistakesOverlay) {
      mistakesOverlay.remove();
    }
    if (this.messageHandler) {
      window.removeEventListener('message', this.messageHandler);
      this.messageHandler = null;
    }
    this.quizAnswered = false;
  }

  private showQuizWindow(promptData: any) {
    // If the iframe already exists on screen, just post message directly to update its content!
    const existingIframe = document.getElementById('quiz-iframe') as HTMLIFrameElement;
    if (existingIframe && existingIframe.contentWindow) {
      this.currentQuizId = promptData.quiz_id;
      this.quizAnswered = false;
      existingIframe.contentWindow.postMessage({
        type: 'quiz-presented',
        data: promptData
      }, '*');
      return;
    }

    this.clearQuiz();
    this.currentQuizId = promptData.quiz_id;

    const parent = document.getElementById('game-container') || document.body;

    // Create the overlay backdrop div
    const overlay = document.createElement('div');
    overlay.id = 'quiz-overlay';
    overlay.style.position = 'absolute';
    overlay.style.top = '0';
    overlay.style.left = '0';
    overlay.style.width = '100%';
    overlay.style.height = '100%';
    overlay.style.backgroundColor = 'transparent';
    overlay.style.zIndex = '9999';

    // Create the iframe pointing to the Next.js quiz overlay page
    const iframe = document.createElement('iframe');
    iframe.id = 'quiz-iframe';
    iframe.style.position = 'absolute';
    iframe.style.top = '0';
    iframe.style.left = '0';
    iframe.style.width = '100%';
    iframe.style.height = '100%';
    iframe.style.border = 'none';
    iframe.style.backgroundColor = 'transparent';

    // Build query parameters
    const params = new URLSearchParams();
    params.set('quiz_id', promptData.quiz_id);
    params.set('question', promptData.question_markdown);
    params.set('options', JSON.stringify(promptData.options_markdown || []));
    params.set('type', promptData.quiz_type === 'true_false' ? 'tf' : 'mcq');

    iframe.src = `${window.location.origin}/quiz-overlay?${params.toString()}`;
    overlay.appendChild(iframe);
    parent.appendChild(overlay);

    // Register a secure postMessage listener for child interactions
    this.messageHandler = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;

      const packet = event.data;
      if (packet.type === 'quiz-submit') {
        this.submitAnswer(packet.index);
      } else if (packet.type === 'quiz-close') {
        this.clearQuiz();
      } else if (packet.type === 'quiz-next') {
        this.requestQuiz(true);
      }
    };
    window.addEventListener('message', this.messageHandler);
  }

  private submitAnswer(index: number) {
    if (this.quizAnswered) return;
    this.quizAnswered = true;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({
        type: 'game.quiz.answer',
        data: {
          quiz_id: this.currentQuizId,
          selected_index: index
        }
      }));
    }
  }

  private handleQuizResult(resultData: any) {
    // Send the result down to the iframe overlay
    const iframe = document.getElementById('quiz-iframe') as HTMLIFrameElement;
    if (iframe && iframe.contentWindow) {
      iframe.contentWindow.postMessage({
        type: 'quiz-result',
        data: resultData
      }, '*');
    }
    if (resultData && resultData.correct) {
      this.hideQuizPrompt();
    }
  }

  private showPauseWindow() {
    // If a quiz overlay is currently displayed, do not pause or open overlapping windows
    if (document.getElementById('quiz-overlay') || this.pauseWindowOpen) {
      return;
    }
    this.pauseWindowOpen = true;
    this.tweens.pauseAll();
    this.anims.pauseAll();

    // Send pause message to the websocket
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'game.pause' }));
    }

    // 1. Sleek semi-transparent black backdrop overlay covering the entire possible viewport
    const pauseBackdrop = this.add.graphics();
    pauseBackdrop.fillStyle(0x000000, 0.65);
    pauseBackdrop.fillRect(-2000, -2000, 6000, 5000);
    // Block all clicks/pointer inputs below the overlay
    pauseBackdrop.setInteractive(new Phaser.Geom.Rectangle(-2000, -2000, 6000, 5000), Phaser.Geom.Rectangle.Contains);
    pauseBackdrop.setDepth(100);

    // 2. Main Orange Square Container Box (500x420) using NineSlice for more vertical spacing
    const pauseDialog = this.add.nineslice(960, 540, 'box_orange_square', undefined, 500, 420, 64, 64, 64, 64)
      .setDepth(101);

    // 3. Header text: PAUSED (using a deep brown color matching the theme beautifully)
    const pauseTitle = this.add.text(960, 390, 'PAUSED', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '56px',
      color: '#451a03'
    }).setOrigin(0.5).setDepth(102);

    // 4. RESUME button using ButtonText_Small_Blue_Round.svg (btn_blue_round)
    const resumeBtn = this.add.sprite(960, 495, 'btn_blue_round')
      .setScale(1.1)
      .setDepth(102)
      .setInteractive({ useHandCursor: true });
    const resumeLabel = this.add.text(960, 495, 'RESUME', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '24px',
      color: '#ffffff'
    }).setOrigin(0.5).setDepth(103);

    resumeBtn.on('pointerover', () => {
      resumeBtn.setScale(1.18);
      resumeLabel.setScale(1.08).setColor('#fef3c7');
    });
    resumeBtn.on('pointerout', () => {
      resumeBtn.setScale(1.1);
      resumeLabel.setScale(1.0).setColor('#ffffff');
    });

    resumeBtn.on('pointerdown', () => {
      // Send resume message to websocket
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'game.resume' }));
      }
      

      this.tweens.resumeAll();
      this.anims.resumeAll();

      // Cleanup all pause window components cleanly
      pauseBackdrop.destroy();
      pauseDialog.destroy();
      pauseTitle.destroy();
      resumeBtn.destroy();
      resumeLabel.destroy();
      exitBtn.destroy();
      exitLabel.destroy();
      this.pauseWindowOpen = false;
    });

    // 5. EXIT button using ButtonText_Small_Blank_Round.svg (btn_blank_round) with black text
    const exitBtn = this.add.sprite(960, 620, 'btn_blank_round')
      .setScale(1.1)
      .setDepth(102)
      .setInteractive({ useHandCursor: true });
    const exitLabel = this.add.text(960, 620, 'EXIT GAME', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '24px',
      color: '#000000'
    }).setOrigin(0.5).setDepth(103);

    exitBtn.on('pointerover', () => {
      exitBtn.setScale(1.18);
      exitLabel.setScale(1.08);
    });
    exitBtn.on('pointerout', () => {
      exitBtn.setScale(1.1);
      exitLabel.setScale(1.0);
    });

    exitBtn.on('pointerdown', () => {
      // Send same game.exit message as the end-of-game Exit Button
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({
          type: 'game.exit',
          data: { session_id: this.sessionId }
        }));
      }
      setTimeout(() => {
        const searchParams = new URLSearchParams(window.location.search);
        const documentId = searchParams.get('document_id');
        const chapterId = searchParams.get('chapter_id');
        if (documentId && chapterId) {
          window.location.href = `/dashboard/games/${documentId}/chapters/${chapterId}`;
        } else {
          window.location.href = '/dashboard';
        }
      }, 150);
    });
  }
}
