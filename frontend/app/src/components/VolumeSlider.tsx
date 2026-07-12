"use client"

import Image from "next/image";
import { useRef, useState } from "react"

type VolumeSliderAssets = {
    volume: number;
    muted: boolean;
    onVolumeChange: (volume: number) => void;
};

export default function VolumeSlider({
    volume,
    muted,
    onVolumeChange
}: VolumeSliderAssets) {
    const track = useRef<HTMLDivElement | null>(null);

    const displayVolume = muted ? 0 : volume;

    const updateVolume = (clientX: number) => {
        if (!track.current) return;

        //RECTRANGLE RANGE
        const rect = track.current.getBoundingClientRect();

        const length = Math.min(clientX - rect.left, rect.width);

        const newVolume = length / rect.width;

        onVolumeChange(newVolume);
    }

    return (
        <div
            //{/* Rectangle volume slider */}
            ref={track}
            onPointerDown={(event) => {
                updateVolume(event.clientX);
                event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                    updateVolume(event.clientX);
                }
            }}
            onPointerUp={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                    event.currentTarget.releasePointerCapture(event.pointerId);
                }
            }}
            onPointerCancel={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                    event.currentTarget.releasePointerCapture(event.pointerId);
                }
            }}
            className="relative h-[20px] w-[160px] cursor-pointer touch-none pointer-events-auto"
        >
            <Image
                src={"/gui/sliders/ScrollBar_Blank_Base.svg"}
                alt="button"
                fill
                draggable={false}
                className="pointer-events-none object-fill"
            />

            <div
                className="pointer-events-none absolute flex inset-0 overflow-hidden"
                style={{ width: `${displayVolume * 100}%` }}
            >
                <div className="relative h-full w-[160px]">
                    <Image
                        src="/gui/sliders/ScrollBar_Blue_Base.svg"
                        alt="filled"
                        fill
                        draggable={false}
                        className="pointer-events-none object-fill"
                    />
                </div>

            </div>

              <div
                className="pointer-events-none absolute top-1/2 h-[32px] w-[32px] -translate-x-1/2 -translate-y-1/2"
                style={{
                    left: `${displayVolume * 100}%`,
                }}
            >
                <Image
                    src="/gui/sliders/ScrollBar_Blank_Button.svg"
                    alt=""
                    fill
                    draggable={false}
                    className="object-contain"
                />
            </div>
        </div>

    )
}




