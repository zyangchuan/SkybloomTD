import Phaser from "phaser";

export function speed(scene : Phaser.Scene) {
    // Speed up the quiz pop up and the animation of the birds or enemies if added in future
    scene.tweens.timeScale = 2;
    scene.anims.globalTimeScale = 2;
}

export function normalSpeed(scene : Phaser.Scene) {
    scene.tweens.timeScale = 1;
     scene.anims.globalTimeScale = 1;
}