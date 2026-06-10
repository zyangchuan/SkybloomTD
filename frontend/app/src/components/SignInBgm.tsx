import { useEffect, useRef } from "react";

export default function SignInBgm() {

    // use a ref to control the audio element
    // this allows us to play/pause the music and reset it when the component unmounts
    const audioRef = useRef<HTMLAudioElement>(null);

    useEffect(() => {
        console.log('SignInBgm component mounted, setting up background music');
        audioRef.current= new Audio('/bgm/paradise_found.mp3');
        const audio = audioRef.current;
        if (audio) {
            audio.loop = true;
            audio.volume = 0.5;
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
    
    return null; // This component doesn't render anything
}
