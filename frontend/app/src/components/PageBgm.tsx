"use client";
import Image from "next/image";

import { useEffect, useState, useRef } from "react";


export default function PageBgm() {

    const audio = useRef<HTMLAudioElement | null>(null);

    // setVolume to be used after adding volumeSlider
    const [volume, setVolume] = useState(0.5);
    const [muted, setMuted] = useState(false);
    const [settingsOpen, setSettingsOpen] = useState(false);

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

    const handleToggleMute = () => {
        if (audio.current) {
            const next = !muted;
            setMuted(next);
            if (next) {
                audio.current.volume = 0; 
            } else {
                audio.current.volume = volume;
            }
        }
        return;
    }

    const handleToggleVolume = (e: React.ChangeEvent<HTMLInputElement>) => {
        const newVolume = parseFloat(e.target.value);
        setVolume(newVolume);
        if (audio.current) {
            audio.current.volume = newVolume;
        } 

        if (newVolume == 0) {
            setMuted(true);
        } else {
            setMuted(false);
        }
    }

    return (
       <div className="fixed! top-6 left-6 z-50!">
            <button
                type="button"
                onClick={() => setSettingsOpen((prev) => !prev)}
                className="relative h-12 w-12 border-0 bg-transparent p-0 cursor-pointer"
                aria-label="Open settings"
            >

                <Image
                    src="/gui/buttons_icons/IconButton_Small_Orange_Rounded.svg"
                    alt=""
                    fill
                    className="object-contain"
                />

                <Image
                    src="/gui/icons/Icon_Large_Menu_Blank.svg"
                    alt=""
                    width={28}
                    height={28}
                    className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2"
                />
            </button>
            
            {settingsOpen && (
               <div className="mt-2 flex items-center gap-3 rounded-md bg-white/90 px-4 py-3 shadow-md">
                 <Image
                    src={muted? "/gui/icons/Icon_Large_AudioOff_Blank.svg"
                              : "/gui/icons/Icon_Large_Audio_Blank.svg"
                    }
                    alt="audio"
                    onClick={handleToggleMute}
                    width={24}
                    height={24}
                    className="cursor-pointer"
                    />
                 <input 
                    type="range"
                    min="0"
                    max="1"
                    step="0.1"
                    value={muted? 0 : volume}
                    onChange={handleToggleVolume}
                    className="w-24"
                    />
                  </div>  

            )}
        </div>
    );




}