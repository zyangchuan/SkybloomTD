export class BgmManager {
    private scene: Phaser.Scene;
    private currentBgmKey: string | null = null;

    constructor(scene: Phaser.Scene) {
        this.scene = scene;
    }

    // If the same bgm is already playing, do nothing. Otherwise, stop current bgm and play the new one.
    play(k: string) {
        if (this.currentBgmKey === k) {
            return;
        }

        if (this.currentBgmKey) {
            this.scene.sound.stopByKey(this.currentBgmKey);
        }

        this.scene.sound.play(k, { loop: false, volume: 0.5 });
        this.currentBgmKey = k;
    }
    
    stop(key: string) {
        this.scene.sound.stopByKey(key);
        this.currentBgmKey = null;
    }

    updateForWave(waveIndex: number) {
        if (waveIndex == 1) {
            this.play('first_wave_bgm');
        } else if (waveIndex == 2) {
            this.play('mid_wave_bgm');
        } else {
            this.play('end_wave_bgm');
        }

    }
}