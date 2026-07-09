// AudioSettings.ts

//the sfx and bgm volume are only from range 0 to 1
export function clamp01(volume: number): number {
        return Math.max(0, Math.min(1, volume));
}
    
export function readVolume(key: string, fallback: number = 0.5): number {
        if (typeof window === 'undefined') {
            console.warn(`localStorage is not available. Returning default volume for key ${key}.`);
            return fallback;
        }

        try {
            const raw = window.localStorage.getItem(key);
            const volume = parseFloat(raw || fallback.toString());
            return clamp01(volume);
        } catch (e) {
            console.error(`Error reading volume from localStorage for key ${key}:`, e);
            return fallback;
        }
}

export function writeVolume(key: string, volume: number) {
        if (typeof window === 'undefined') {
            console.warn(`localStorage is not available. Cannot write volume for key ${key}.`);
            return;
        }
        try {
            window.localStorage.setItem(key, volume.toString());
        } catch (e) {
            console.error(`Error writing volume to localStorage for key ${key}:`, e);
        }
}

export const AudioSettings = {
    getBgmVolume() {
        return readVolume('bgm_volume', 0.5);
    },

    setBgmVolume(volume: number) {
        writeVolume('bgm_volume', clamp01(volume));
    },

    getSfxVolume() {
        return readVolume('sfx_volume', 0.5);
    },

    setSfxVolume(volume: number) {
        writeVolume('sfx_volume', clamp01(volume));
    }
}

export default AudioSettings;
