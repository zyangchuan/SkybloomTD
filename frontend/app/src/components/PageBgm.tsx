import { useEffect, useRef, useState } from "react";
import Image from "next/image";

export default function PageBgm({ src }: { src: string }) {

     // use a ref to control the audio element
        // this allows us to play/pause the music and reset it when the component unmounts
        const audioRef = useRef<HTMLAudioElement>(null);
        const [volume, setVolume] = useState(0.5); // You can adjust the initial volume here
        const [muted, setMuted] = useState(false);
    
        useEffect(() => {
            console.log('SignInBgm component mounted, setting up background music');
            audioRef.current= new Audio(src);
            const audio = audioRef.current;
            if (audio) {
                audio.loop = true;
                audio.volume = volume;
                const startMusic = () => {
                    console.log('Starting background music');
                    audio.play().catch((error) => {
                        console.error('Error playing audio:', error);
                    });
                    // remove the click listener after starting the music to prevent multiple triggers
                    window.removeEventListener('click', startMusic);
                }
    
                window.addEventListener('click', startMusic);
                
                // Cleanup function to pause the music when the component unmounts
                return () => {
                    audio.pause();
                    window.removeEventListener('click', startMusic);
                };
            }
        }, []);
    
        // Optional: You can add a button to toggle mute/unmute
        const handleVolumeToggle = () => {
            if (audioRef.current) {
                // if true, it is currently muted, so unmute it. If false, it is currently unmuted, so mute it.
                if (!muted) {
                    audioRef.current.volume = 0;
                    setMuted(true);
                } else {
                    audioRef.current.volume = volume;
                    setVolume(audioRef.current.volume);
                    setMuted(false);
                }
            }
        };
    
        // You can add a volume control slider and bind it to the audio element
        const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
            const newVolume = parseFloat(e.target.value);
            setVolume(newVolume);
            if (audioRef.current) {
                audioRef.current.volume = newVolume;
            }
            if (newVolume === 0) {
                setMuted(true);
            } else {
                setMuted(false);
            }
        };
    
        // return a volume control slider for demonstration purposes
        return (
            <div className="fixed bottom-4 right-4 flex items-center gap-2  rounded-md px-3 py-2">
                <Image 
                    src={muted ? "/gui/icons/Icon_Large_AudioOff_Grey.svg" : "/gui/icons/Icon_Large_Audio_Grey.svg"} 
                    alt="music" width={24} height={24} 
                    className="cursor-pointer"
                    onClick={handleVolumeToggle} />
                <input
                    type="range"
                    min="0"
                    max="1"
                    step="0.01"
                    value={muted? 0 : volume}
                    onChange={handleVolumeChange}
                    className="w-24 accent-yellow-400"
                />
            </div>
        )
    }
    