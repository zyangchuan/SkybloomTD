import Phaser from 'phaser';

export class VolumeSlider {
    private track: Phaser.GameObjects.Image;
    private thumb: Phaser.GameObjects.Image;
    private trackResidue : Phaser.GameObjects.Image;

    constructor(scene: Phaser.Scene, x: number, y: number, onVolumeChange: (volume: number) => void) {
        this.track = scene.add.image(x, y, 'icon_blank_volume_track').setOrigin(0.5, 0.5).setDepth(102).setScale(0.4);
        this.track.setInteractive({ useHandCursor : true});


        //Need updateVolume, even when it is closed or window is refreshed, the sound remains the same
        const savedVolume = Number(localStorage.getItem('volume') ?? 0.5);

        // set it as the residue of the track
        this.trackResidue = scene.add.image(x, y, 'icon_blue_volume_track').setOrigin(0.5, 0.5).setDepth(104).setScale(0.4);
        this.trackResidue.setCrop(0, 0, savedVolume * this.trackResidue.width, y);

        const left  = x - this.track.displayWidth / 2;
        const right = x + this.track.displayWidth / 2;

        this.thumb = scene.add.image(left + this.track.displayWidth * savedVolume, y, 'icon_volume_thumb')
            .setOrigin(0.5).setDepth(105).setScale(0.65)
            .setInteractive({ draggable: true });

        scene.input.setDraggable(this.thumb);


        // Need the information of the input of the pointer to indicate the coordinate that where the button needs to go
        this.track.on('pointerdown', (pointer : Phaser.Input.Pointer) => {
            const posX = Phaser.Math.Clamp(pointer.x, left, right);

            this.thumb.x = posX;
            const volume = (posX - left) / (right - left);
            onVolumeChange(volume);

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            localStorage.setItem('volume', volume.toString());
        });

        this.thumb.on('drag', (_pointer: any, dragX: number) => {
            this.thumb.x = Phaser.Math.Clamp(dragX, left, right);
            const volume = (this.thumb.x - left) / (right - left);
            onVolumeChange(volume);

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            localStorage.setItem('volume', volume.toString());
        });
    }

    destroy() {
        this.track.destroy();
        this.thumb.destroy();
        this.trackResidue.destroy();
    }

    setDepth(depth: number) {
        this.track.setDepth(depth);
        this.thumb.setDepth(depth + 1);
        return this;
    }
}
