import Phaser from 'phaser';

export class VolumeSlider {
    private track: Phaser.GameObjects.Image;
    private thumb: Phaser.GameObjects.Image;

    constructor(scene: Phaser.Scene, x: number, y: number, onVolumeChange: (volume: number) => void) {
        this.track = scene.add.image(x, y, 'icon_volume_track').setOrigin(0.5).setDepth(102);

        const left  = x - this.track.displayWidth / 2;
        const right = x + this.track.displayWidth / 2;

        this.thumb = scene.add.image(right, y, 'icon_volume_thumb')
            .setOrigin(0.5).setDepth(103)
            .setInteractive({ draggable: true });

        scene.input.setDraggable(this.thumb);

    
        this.thumb.on('drag', (_pointer: any, dragX: number) => {
            this.thumb.x = Phaser.Math.Clamp(dragX, left, right);
            const volume = (this.thumb.x - left) / (right - left);
            onVolumeChange(volume);
        });
    }

    destroy() {
        this.track.destroy();
        this.thumb.destroy();
    }

    setDepth(depth: number) {
        this.track.setDepth(depth);
        this.thumb.setDepth(depth + 1);
        return this;
    }
}
