import Phaser from 'phaser';
import AudioSettings from './AudioSettings';

export class BgmManager {
    private currentSong: Phaser.Sound.WebAudioSound | null = null;
    private maxVolume = AudioSettings.getBgmVolume();
    private fadeDuration = 2000; 

    constructor(private scene: Phaser.Scene) {}

 play(song: string) {
        // Already playing this song — don't restart it.
        if (this.currentSong?.key === song && this.currentSong.isPlaying) {
            return;
        }

        const previous = this.currentSong;

        if (previous) {
            // fade the old song OUT, then start the new one.
            // tweens  take charge of music volume
            this.scene.tweens.add({
                targets: previous,
                volume: 0,
                duration: this.fadeDuration,
                ease: 'Linear',
                onComplete: () => {
                    previous.stop();
                    previous.destroy();
                    this.fadeIn(song, false);   // fade the new song IN
                },
            });
        } else {
            this.fadeIn(song, true);
        }
    }

    // Starts a song silent and fades it UP to full volume.
    private fadeIn(song: string, start: boolean) {
        const next = this.scene.sound.add(song, {
            loop: true,
            volume: start === true? this.maxVolume : 0,
        }) as Phaser.Sound.WebAudioSound;
        next.play();
        this.currentSong = next;
        
        if (!start) {
            this.scene.tweens.add({
                targets: next,
                volume: this.maxVolume,
                duration: this.fadeDuration,
                ease: 'Linear',
            });
        }
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
       this.maxVolume = volume;
         if (this.currentSong) {
            this.currentSong.setVolume(volume);
         }
         AudioSettings.setBgmVolume(volume);
    }

    updateForWave(wave: number) {

        if (wave <= 1) {
            this.play('audio_wave_1');
        }
        if (wave == 2) {
            this.play('audio_wave_2');
        }
        if (wave >= 3) {
            this.play('audio_wave_3');
        }  
    }

}