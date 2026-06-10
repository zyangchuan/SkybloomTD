export class BgmManager {
    private scene: Phaser.Scene;
    private currentBgmKey: string | null = null;

    constructor(scene: Phaser.Scene) {
        this.scene = scene;
    }

    // If the same bgm is already playing, do nothing. Otherwise, stop current bgm and play the new one.
    play(song : string) {
        if (this.currentBgmKey === song) {
            return;
        }

        if (this.currentBgmKey) {
            this.scene.sound.stopByKey(this.currentBgmKey);
        }
        
        this.scene.sound.play(song, { loop: true, volume: 0.5 });
        this.currentBgmKey = song;
    }
    
    stop() {
        if (!this.currentBgmKey) {
            return;
        }
        const key: string = this.currentBgmKey;
        this.scene.sound.stopByKey(key);
        this.currentBgmKey = null;
    }


    // update the bgm based on the current wave index. For wave 1 play first_wave_bgm, for wave 2 play mid_wave_bgm, and for wave 3+ play end_wave_bgm.
    updateForWave(waveIndex: number) {
        if (waveIndex <= 1) {
            this.play('front_game_bgm');
        } else if (waveIndex == 2) {
            this.play('middle_game_bgm');
        } else {
            this.play('end_game_bgm');
        }
    }

    reset() {
        this.stop();
        this.currentBgmKey = null;
    }
}