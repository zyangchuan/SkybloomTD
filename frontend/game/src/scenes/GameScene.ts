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
  private gridWidth: number = 18;
  private gridHeight: number = 12;
  private tileSize: number = 0;
  private offsetX: number = 0;
  private offsetY: number = 0;
  private enemyPath: any[] = [];
  private obstacles: any[] = [];

  // Drag and placement elements
  private activeDragSprite: Phaser.GameObjects.Sprite | null = null;
  private activeDragBirdType: string | null = null;
  private gridHighlightGraphics: Phaser.GameObjects.Graphics | null = null;
  private closestCellHighlight: Phaser.GameObjects.Graphics | null = null;
  private pulseTween: Phaser.Tweens.Tween | null = null;

  constructor() {
    super('GameScene');
  }

  create(data: { initialState: any, ws: WebSocket, levelId: string }) {
    this.ws = data.ws;
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
      
      this.add.image(posX, posY, spriteKey)
        .setDisplaySize(this.tileSize + 4, this.tileSize + 4)
        .setDepth(1);
    });

    // 4. Render sleek, premium top-left HUD panel using box_orange_square as a NineSlice container
    this.add.nineslice(360, 69, 'box_orange_square', undefined, 700, 90, 32, 32, 32, 32).setDepth(30);

    // Health Icon & Text
    this.add.image(62, 69, 'icon_heart').setDisplaySize(50, 50).setDepth(30);
    this.healthText = this.add.text(124, 69, '100', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#f87171',
    }).setOrigin(0, 0.5).setDepth(30);

    // Essence Icon & Text
    this.add.image(274, 69, 'icon_essence').setDisplaySize(50, 50).setDepth(30);
    this.essenceText = this.add.text(336, 69, '0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#fbbf24',
    }).setOrigin(0, 0.5).setDepth(30);

    // Wave Text (No Icon)
    this.waveText = this.add.text(484, 69, 'WAVE 0', {
      fontFamily: '"Concert One", system-ui, sans-serif',
      fontSize: '54px',
      color: '#38bdf8',
    }).setOrigin(0, 0.5).setDepth(30);

    // 5. Selectable Birds Tray Panel at Bottom Center using box_orange_square as NineSlice outer container
    this.add.nineslice(960, 1152, 'box_orange_square', undefined, 760, 170, 32, 32, 32, 32).setDepth(30);

    const birds = ['sparrow', 'woodpecker', 'eagle', 'peacock'];
    const startX = 637;
    const boxY = 1152;
    const boxSize = 136;
    const headSize = 98;

    const birdStats: Record<string, { damage: number, range: number, fireRate: string, attack: string, cost: number, color: string }> = {
      sparrow: { damage: 10, range: 3.5, fireRate: '1.0/s', attack: 'SINGLE', cost: 50, color: '#38bdf8' },
      woodpecker: { damage: 6, range: 3.5, fireRate: '2.0/s', attack: 'SINGLE', cost: 65, color: '#fb7185' },
      eagle: { damage: 30, range: 6.0, fireRate: '0.4/s', attack: 'SINGLE', cost: 130, color: '#fb923c' },
      peacock: { damage: 7, range: 3.5, fireRate: '1.0/s', attack: 'SPLASH', cost: 90, color: '#c084fc' }
    };

    birds.forEach((bird, index) => {
      const boxX = startX + index * 170 + boxSize / 2;

      // 1. Create Tooltip Container
      const tooltipContainer = this.add.container(boxX, boxY - 230).setDepth(35);
      tooltipContainer.setVisible(false);

      // Background plate using Box_Square NineSlice
      const tooltipBg = this.add.nineslice(0, 0, 'box_square', undefined, 300, 220, 32, 32, 32, 32);
      tooltipContainer.add(tooltipBg);

      // Title/Header
      const statsInfo = birdStats[bird];
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

      // Spawn temporary visual placement preview
      this.activeDragSprite = this.add.sprite(pointer.x, pointer.y, `tower_${birdType}`)
        .setAlpha(0.8)
        .setDepth(40);
      
      const dragScale = this.tileSize / this.activeDragSprite.width * 1.5;
      this.activeDragSprite.setScale(dragScale);

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

          if (message.type === 'game.state' || message.type === 'game.session.started') {
            this.updateHUD(message.data);
          } else if (message.type === 'game.action.rejected') {
            this.showRejectMessage(message.data?.error || 'ACTION REJECTED');
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
    });
  }

  /**
   * Slowly spin all active towers inside the Phaser game update loop
   */
  update() {
    this.towers.forEach((tower) => {
      tower.update();
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
      const { id, type, position } = birdData;
      activeIds.add(id);

      const gridX = position.x;
      const gridY = position.y;

      if (!this.towers.has(id)) {
        const posX = this.offsetX + gridX * this.tileSize + this.tileSize / 2;
        const posY = this.offsetY + gridY * this.tileSize + this.tileSize / 2;
        
        const tower = new Tower(this, posX, posY, id, type, gridX, gridY);
        
        // Retain original aspect ratio perfectly by scaling proportionally to fit the tile size!
        const towerScale = this.tileSize / tower.width * 1.3;
        tower.setScale(towerScale);
        tower.setDepth(4); // Keep towers layered appropriately
        
        this.towers.set(id, tower);
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
    if (gameState.birds !== undefined) {
      this.syncTowers(gameState.birds);
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
