"use client";

import { useEffect, useState, useRef } from "react";


export default function PageBgm() {

    const audio = useRef<HTMLAudioElement | null>(null);

    // setVolume to be used after adding volumeSlider
    const [volume, _setVolume] = useState(0.5);

    useEffect(() => {
        // to not have music in quiz interface 
        if (window.self !== window.top) return;
        console.log('setting up background music');
        audio.current = new Audio('/audio/Sign_In_Music_goofy_loop.mp3');

        if (audio.current) {
            audio.current.loop = true;
            audio.current.volume = volume;


            const startMusic = () => {
                if (!audio.current) {
                    console.error('no audio can be played');
                    return;
                }
                if (audio.current) {
                    audio.current.play().catch(e => {
                        return console.log('audio can not be played');
                    });
                }
                window.removeEventListener('keydown', startMusic);
                window.removeEventListener('click', startMusic);
                window.removeEventListener('touchstart', startMusic);

            }

            startMusic();

            //add event to invoke the music, it possibly can not be played
            window.addEventListener('keydown', startMusic);
            window.addEventListener('click', startMusic);
            window.addEventListener('touchstart', startMusic);

            return () => {
                if (audio.current) {
                    audio.current.pause();
                    window.removeEventListener('keydown', startMusic);
                    window.removeEventListener('click', startMusic);
                    window.removeEventListener('touchstart', startMusic);
                    return;
                }
                console.error('no audio can be played');
            }

        }

    }, []);

    return null;


}