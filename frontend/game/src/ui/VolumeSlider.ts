import Phaser from 'phaser';

export class VolumeSlider {
    private track: Phaser.GameObjects.Image;
    private thumb: Phaser.GameObjects.Image;
    private trackResidue : Phaser.GameObjects.Image;
    private audio : Phaser.GameObjects.Image;

    constructor(scene: Phaser.Scene, x: number, y: number, onVolumeChange: (volume: number) => void) {
        this.track = scene.add.image(x, y, 'icon_blank_volume_track').setOrigin(0.5, 0.5).setDepth(102).setScale(0.4);
        this.track.setInteractive({ useHandCursor : true});

      
        //Need updateVolume, even when it is closed or window is refreshed, the sound remains the same
        const savedVolume = Number(localStorage.getItem('volume') ?? 0.5);
        const savedMuted = (localStorage.getItem('muted')) === 'true' || savedVolume === 0;

        const savedPos = Number(localStorage.getItem('position')?? this.track.x);

        this.audio = scene.add.image(this.track.x - 130, this.track.y - 110,savedMuted? 'icon_audio_off' : 'icon_audio_on').setScale(0.35).setDepth(110).setInteractive({ userHandCursor: true });


        // set it as the residue of the track
        this.trackResidue = scene.add.image(x, y, 'icon_blue_volume_track').setOrigin(0.5, 0.5).setDepth(104).setScale(0.4);
        this.trackResidue.setCrop(0, 0, (savedMuted? 0 : savedVolume) * this.trackResidue.width, y);

        const left  = x - this.track.displayWidth / 2;
        const right = x + this.track.displayWidth / 2;

        this.thumb = scene.add.image(left + this.track.displayWidth * (savedMuted? 0 : savedVolume) , y, 'icon_volume_thumb')
            .setOrigin(0.5).setDepth(105).setScale(0.65)
            .setInteractive({ draggable: true });

        scene.input.setDraggable(this.thumb);

        this.audio.on('pointerover', () => this.audio.setScale(0.37));
        this.audio.on('pointerout', () => this.audio.setScale(0.35));
        this.audio.on('pointerdown', () => {
            const muted = (localStorage.getItem('muted') === 'true');

            localStorage.setItem('muted', (!muted).toString());

            if ((localStorage.getItem('muted')) === 'true') {
                this.thumb.x = Phaser.Math.Clamp(this.track.x - left, left, right);
                this.audio.setTexture('icon_audio_off');
                onVolumeChange(0);
                this.thumb.x = left;
                this.trackResidue.setCrop(0, 0, 0 * this.trackResidue.width, y);
            } else {
                this.thumb.x = Phaser.Math.Clamp(this.track.x - left, left, right);
                this.audio.setTexture('icon_audio_on');
                onVolumeChange(savedVolume);
                this.thumb.x = savedPos;
                this.trackResidue.setCrop(0, 0, savedVolume * this.trackResidue.width, y);      
            }

        });

        // Need the information of the input of the pointer to indicate the coordinate that where the button needs to go
        this.track.on('pointerdown', (pointer : Phaser.Input.Pointer) => {
            const posX = Phaser.Math.Clamp(pointer.x, left, right);

            this.thumb.x = posX;
            const volume = (posX - left) / (right - left);
            onVolumeChange(volume);

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            localStorage.setItem('volume', volume.toString());
            if (this.thumb.x === left) {
                this.audio.setTexture('icon_audio_off');
                localStorage.setItem('muted', 'true');
            } else {
                this.audio.setTexture('icon_audio_on');
                localStorage.setItem('muted', 'false');
            }
            localStorage.setItem('position', this.thumb.x.toString())
        });

        this.thumb.on('drag', (_pointer: any, dragX: number) => {
            this.thumb.x = Phaser.Math.Clamp(dragX, left, right);
            const volume = (this.thumb.x - left) / (right - left);
            onVolumeChange(volume);

            if (this.thumb.x === left) {
                this.audio.setTexture('icon_audio_off');
                localStorage.setItem('muted', 'true');
            } else {
                this.audio.setTexture('icon_audio_on');
                localStorage.setItem('muted', 'false');
            }

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            localStorage.setItem('volume', volume.toString());
            localStorage.setItem('position', this.thumb.x.toString());
        });
    }

    destroy() {
        this.track.destroy();
        this.thumb.destroy();
        this.trackResidue.destroy();
        this.audio.destroy();
    }

    setDepth(depth: number) {
        this.track.setDepth(depth);
        this.thumb.setDepth(depth + 1);
        return this;
    }
}
