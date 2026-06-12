import Phaser from 'phaser';

export function restartGame(scene: Phaser.Scene, ws: WebSocket | null, sessionId: string) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'game.exit', data: { session_id: sessionId } }));
  }
  scene.scene.restart();
}