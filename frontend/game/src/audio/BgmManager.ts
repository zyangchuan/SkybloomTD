import Phaser from 'phaser';

export class BgmManager {
    private currentSong: Phaser.Sound.WebAudioSound | null = null;

    constructor(private scene: Phaser.Scene) {}

    play(song: string) {
        if (this.currentSong) {
            this.currentSong.stop();
            this.currentSong.destroy();
        }

        this.currentSong = this.scene.sound.add(song, {
            loop: true,
            volume: 0.5
        }) as Phaser.Sound.WebAudioSound;

        this.currentSong.play();
    }

    /*
     * pause -> freeze the moment, and start from the moment when reusme
     * stop -> restart the audio
    */

    pause() {
        if (this.currentSong) {
            this.currentSong.pause();
        }
    }

    stop() {
        if (this.currentSong) {
            this.currentSong.stop();
        }
    }

    resume() {
        if (this.currentSong) {
            this.currentSong.resume();
        }
    }

    setVolume(volume: number) {
        if (this.currentSong) {
            this.currentSong.setVolume(volume);
        }
    }

    updateForWave(wave: number) {

        if (wave <= 1) {
            this.play('audio_wave_1');
        }
        if (wave == 2) {
            this.play('audio_wave_1');
        }
        if (wave >= 3) {
            this.play('audio_wave_1');
        }  
    }

}