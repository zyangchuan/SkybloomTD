"use client";

import Image from "next/image";
import { useRouter } from "next/navigation";

export function AlmanacButton() {
    const route = useRouter();

    return (
        <button
            onClick={() => route.push("/almanac")}
            className="animate-float transition-transform duration-20 hover:scale-105 active:scale-95"
            aria-label="Open SkyBloomTD Almanac"
        >
            <Image
                src="/almanac.png"
                alt="Open SkyBloomTD Almanac"
                width={220}
                height={220}
                className="object-contain cursor-pointer"
            />
        </button>
    );
}