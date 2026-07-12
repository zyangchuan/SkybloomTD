"use client";
import Image from "next/image";
import VolumeSlider from "./VolumeSlider";
import { useEffect, useState, useRef } from "react";


export default function PageAudio() {

    const audio = useRef<HTMLAudioElement | null>(null);

    // setBgmVolume to be used after adding bgmVolumeSlider
    const [bgmVolume, setBgmVolume] = useState(0.5);
    const [bgmMuted, setBgmMuted] = useState(false);
    const [sfxVolume, setSfxVolume] = useState(0.5);
    const [sfxMuted, setSfxMuted] = useState(false);
    const [settingsOpen, setSettingsOpen] = useState(false);

    useEffect(() => {
        // to not have music in quiz interface 
        if (window.self !== window.top) return;
        console.log('setting up background music');


        const savedBgmVolume = Number(
            window.localStorage.getItem("web_bgm_volume") ?? "0.5"
        );

        const savedBgmMuted =
            window.localStorage.getItem("web_bgm_mute") === "true";

        const savedSfxVolume = Number(
            window.localStorage.getItem("web_sfx_volume") ?? "0.5"
        );

        const savedSfxMuted =
            window.localStorage.getItem("web_sfx_mute") === "true";

        setBgmVolume(savedBgmVolume);
        setBgmMuted(savedBgmMuted);
        setSfxVolume(savedSfxVolume);
        setSfxMuted(savedSfxMuted);

        audio.current = new Audio('/audio/Sign_In_Music_goofy_loop.mp3');

        if (audio.current) {
            audio.current.loop = true;
            audio.current.volume = bgmVolume;


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

    const handleToggleBgmMute = () => {
        if (audio.current) {
            const next = !(bgmMuted);
            setBgmMuted(next);

            window.localStorage.setItem("web_bgm_mute", String(next));

            if (next) {
                audio.current.volume = 0;
            } else {
                audio.current.volume = bgmVolume;
            }
        }
        return;
    }

    const handleToggleSfxMute = () => {
        const next = !(sfxMuted);
        setSfxMuted(next);
        window.localStorage.setItem("web_sfx_mute", String(next));
        return;
    }

    return (
        <div className="fixed top-6 left-6 !z-50">
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
                <div className="absolute">
                    <div className="relative h-66 w-66">
                        <Image
                            src="/gui/boxes_banners/Box_Orange_Square.svg"
                            alt="s"
                            fill
                            className="object-fill pointer-events-none"
                        />

                        <p className="absolute inset-0 flex justify-center translate-y-[10px] font-bold text-[30px] text-[#451a03]">
                            SETTINGS
                        </p>

                        {/*BGM volume*/}
                        <div className="absolute inset-0 z-20 flex -translate-y-[20px] items-center justify-center gap-2 pointer-events-none">

                            <p className="absolute -translate-y-[30px] font-bold text-[25px] text-[#451a03]">
                                BGM
                            </p>

                            <Image
                                src={bgmMuted ? "/gui/icons/Icon_Large_MusicOff_Blank.svg"
                                    : "/gui/icons/Icon_Large_Music_Blank.svg"
                                }
                                alt="audio"
                                onClick={handleToggleBgmMute}
                                width={24}
                                height={24}
                                className="w-12 h-12 pointer-events-auto cursor-pointer"
                            />

                            <div className="scale-[0.9]">
                                <VolumeSlider
                                    volume={bgmVolume}
                                    muted={bgmMuted}
                                    onVolumeChange={(newVolume) => {
                                        setBgmVolume(newVolume);
                                        setBgmMuted(newVolume === 0);

                                        window.localStorage.setItem(
                                            "web_bgm_volume",
                                            String(newVolume)
                                        );

                                        window.localStorage.setItem(
                                            "web_bgm_mute",
                                            String(newVolume === 0)
                                        );

                                        if (audio.current) {
                                            audio.current.volume = newVolume;
                                        }
                                    }}
                                />
                            </div>

                        </div>

                        {/*SFX Volume*/}
                        <div className="absolute inset-0 z-10 flex  items-center justify-center translate-y-[70px] gap-2 pointer-events-none">

                            <p className="absolute -translate-y-[30px] font-bold text-[25px] text-[#451a03]">
                                SFX
                            </p>
                            <Image
                                src={sfxMuted ? "/gui/icons/Icon_Large_AudioOff_Blank.svg"
                                    : "/gui/icons/Icon_Large_Audio_Blank.svg"
                                }
                                alt="audio"
                                onClick={handleToggleSfxMute}
                                width={24}
                                height={24}
                                className="w-12 h-12 object-contain pointer-events-auto cursor-pointer"
                            />
                            <div className="scale-[0.9]">
                                <VolumeSlider
                                    volume={sfxVolume}
                                    muted={sfxMuted}
                                    onVolumeChange={(newVolume) => {
                                        setSfxVolume(newVolume);
                                        setSfxMuted(newVolume === 0);

                                        window.localStorage.setItem(
                                            "web_sfx_volume",
                                            String(newVolume)
                                        );

                                        window.localStorage.setItem(
                                            "web_sfx_mute",
                                            String(newVolume === 0)
                                        );
                                    }}
                                />
                            </div>

                        </div>
                    </div>
                </div>
            )}
        </div>
    );




}