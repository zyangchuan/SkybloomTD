import Phaser from 'phaser';
import { AudioSettings } from '../audio/AudioSettings'; 

const mutedSize = 0.44;
const unmutedSize = 0.44;

export class VolumeSlider {
    private track: Phaser.GameObjects.Image;
    private thumb: Phaser.GameObjects.Image;
    private trackResidue : Phaser.GameObjects.Image;
    private audio : Phaser.GameObjects.Image;
    private lastVolume: number;

    constructor(scene: Phaser.Scene, key: string, x: number, y: number, onVolumeChange: (volume: number) => void) {
        this.track = scene.add.image(x, y, 'icon_blank_volume_track').setOrigin(0.5, 0.5).setDepth(102).setScale(0.3);
        this.track.setInteractive({ useHandCursor : true});

      
        //Need updateVolume, even when it is closed or window is refreshed, the sound remains the same
        const savedVolume = key === 'bgm' ? AudioSettings.getBgmVolume() : AudioSettings.getSfxVolume();
        const savedMuted = (window.localStorage.getItem(`${key}` + '_muted')) === 'true' || savedVolume === 0;

        const savedLastVolume = Number(window.localStorage.getItem(`${key}` + '_lastVolume') ?? 0.5);
        this.lastVolume = savedLastVolume > 0 ? savedLastVolume : 0.5;
       
        this.audio = scene.add.image(this.track.x - 280, this.track.y - 10, savedMuted? `icon_audio_${key}_off`: `icon_audio_${key}_on`).setScale(savedMuted ? mutedSize : unmutedSize).setDepth(110).setInteractive({ useHandCursor: true });

        this.trackResidue = scene.add.image(x, y, 'icon_blue_volume_track').setOrigin(0.5, 0.5).setDepth(104).setScale(0.3);
        this.trackResidue.setCrop(0, 0, (savedMuted? 0 : savedVolume) * this.trackResidue.width, y);

        const left  = x - this.track.displayWidth / 2;
        const right = x + this.track.displayWidth / 2;

        this.thumb = scene.add.image(left + this.track.displayWidth * (savedMuted? 0 : savedVolume), y, 'icon_volume_thumb')
            .setOrigin(0.5).setDepth(105).setScale(0.50)
            .setInteractive({ draggable: true });

        scene.input.setDraggable(this.thumb);


        // Audio image is replaced by a default bird 
        this.audio.on('pointerover', () => {
            const muted = (window.localStorage.getItem(`${key}` + '_muted') === 'true');
            this.audio.setScale(muted ? mutedSize + 0.02 : unmutedSize + 0.02);
        });
            
        this.audio.on('pointerout', () => {
            const muted = (window.localStorage.getItem(`${key}` + '_muted') === 'true');
            this.audio.setScale(muted ? mutedSize : unmutedSize);
        });

        this.audio.on('pointerdown', () => {
            const muted = (window.localStorage.getItem(`${key}` + '_muted') === 'true');
            window.localStorage.setItem(`${key}` + '_muted', (!muted).toString());

            if ((window.localStorage.getItem(`${key}` + '_muted')) === 'true') {
                this.thumb.x = Phaser.Math.Clamp(this.track.x - left, left, right);
                this.audio.setTexture(`icon_audio_${key}_off`).setScale(mutedSize);
                onVolumeChange(0);
                this.thumb.x = left;
                this.trackResidue.setCrop(0, 0, 0 * this.trackResidue.width, y);
            } else {
                const pos = left + (right - left) * this.lastVolume;
                this.thumb.x = Phaser.Math.Clamp(this.track.x - left, left, right);
                this.audio.setTexture(`icon_audio_${key}_on`).setScale(unmutedSize);
                onVolumeChange(this.lastVolume);
                this.thumb.x = pos;
                this.trackResidue.setCrop(0, 0, this.lastVolume * this.trackResidue.width, y);      
            }

        });

        // Need the information of the input of the pointer to indicate the coordinate that where the button needs to go
        this.track.on('pointerdown', (pointer : Phaser.Input.Pointer) => {
            const posX = Phaser.Math.Clamp(pointer.x, left, right);

            this.thumb.x = posX;
            const volume = (posX - left) / (right - left);
            onVolumeChange(volume);
            if (volume > 0) {
                this.lastVolume = volume;
                window.localStorage.setItem(`${key}` + '_lastVolume', this.lastVolume.toString());
            }

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            if (this.thumb.x === left) {
                this.audio.setTexture(`icon_audio_${key}_off`);
                window.localStorage.setItem(`${key}` + '_muted', 'true');
            } else {
                this.audio.setTexture(`icon_audio_${key}_on`);
                window.localStorage.setItem(`${key}` + '_muted', 'false');
            }
            window.localStorage.setItem(`${key}` + '_position', this.thumb.x.toString())
        });

        this.thumb.on('drag', (_pointer: any, dragX: number) => {
            this.thumb.x = Phaser.Math.Clamp(dragX, left, right);
            const volume = (this.thumb.x - left) / (right - left);
            onVolumeChange(volume);
            if (volume > 0) {
                this.lastVolume = volume;
                window.localStorage.setItem(`${key}` + '_lastVolume', this.lastVolume.toString());
            }

            if (this.thumb.x === left) {
                this.audio.setTexture(`icon_audio_${key}_off`);
                window.localStorage.setItem(`${key}` + '_muted', 'true');
            } else {
                this.audio.setTexture(`icon_audio_${key}_on`);
                window.localStorage.setItem(`${key}` + '_muted', 'false');
            }

            this.trackResidue.setCrop(0, 0, volume * this.trackResidue.width, y);
            window.localStorage.setItem(`${key}` + '_position', this.thumb.x.toString());
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