"use client";

import Image from "next/image";
import { useState } from "react";

import OrangeSquare from "@/components/OrangeSquare";
import BlueSquare from "@/components/BlueSquare";

type Category = "birds" | "enemies";

export default function AlmanacPage() {
    const [selectedCategory, setSelectedCategory] =
        useState<Category | null>(null);

    return (
        <div className="flex min-h-screen items-center justify-center">
            <OrangeSquare className="relative h-[800px] w-[1000px]">
                <h1 className="absolute left-1/2 top-12 -translate-x-1/2 text-[48px] font-bold">
                    SkyBloomTD Almanac
                </h1>

                {selectedCategory === null && (
                    <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-[100px]">
                        <div className="flex gap-[100px]">
                            <button
                                onClick={() => setSelectedCategory("birds")}
                                className="relative flex h-[300px] w-[400px] items-center"
                            >
                                <BlueSquare className="relative h-full w-full">
                                    <Image
                                        src="/assets/birds/sparrow_head.png"
                                        fill
                                        alt="birds"
                                        className="-translate-y-[40px] scale-[0.6] object-contain"
                                    />

                                    <p className="absolute left-1/2 -translate-x-1/2 translate-y-[180px] text-[42px] font-bold">
                                        BIRDS
                                    </p>
                                </BlueSquare>
                            </button>

                            <button
                                onClick={() => setSelectedCategory("enemies")}
                                className="flex h-[300px] w-[400px] items-center"
                            >
                                <BlueSquare className="relative h-full w-full">
                                    <Image
                                        src="/assets/enemies/smog.png"
                                        fill
                                        alt="enemies"
                                        className="-translate-y-[40px] scale-[0.7] object-contain"
                                    />

                                    <p className="absolute left-1/2 -translate-x-1/2 translate-y-[180px] text-[42px] font-bold">
                                        ENEMIES
                                    </p>
                                </BlueSquare>
                            </button>
                        </div>
                    </div>
                )}
            </OrangeSquare>
        </div>
    );
}