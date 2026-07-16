"use client"

import Image from "next/image";
import OrangeSquare from "@/components/OrangeSquare";
import BlueSquare from "@/components/BlueSquare";
import ButtonWhite from "@/components/ButtonWhite";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { BIRD_INFO, AlmanacEntry}  from "@/data/almanac";

export default function BirdsPage() {

    const route = useRouter();
    const [selectedBird, setSelectedBird] = useState< AlmanacEntry>(BIRD_INFO[0] ?? null);

    return (
        <div className="flex min-h-screen items-center justify-center">

            <OrangeSquare className="relative h-[800px] w-[1000px]">
                <div className="absolute translate-x-[10px]">
                    {/* Back Button */}
                    <ButtonWhite
                        onClick={() => route.back()}
                        className="flex w-[120px] h-[88px] items-center justify-center gap-2"
                    >
                        <ArrowLeft className="w-[20px] h-[15px]" />
                        <span className="text-[22px]">
                            Back
                        </span>
                    </ButtonWhite>
                </div>

                <div className="absolute left-[60px] top-[180px] bottom-[20px] right-[60px]">
                    {/* Main content*/}
                    <div className="grid h-full min-h-0 grid-cols-[480px_1fr]">

                        {/* BIRDS */}
                        <div className="border-r-4 flex flex-col min-h-0 border-orange-900/20">
                            <h1 className="mb-10 text-center font-bold text-[42px]">
                                BIRDS
                            </h1>

                            <div className="grid grid-cols-2 gap-10 min-h-0 justify-items-center overflow-y-auto pb-[20px]">
                                 {/* Birds icon */}
                                {BIRD_INFO.map((bird) => (
                                        <button
                                            key={bird.id}
                                            onClick={() => setSelectedBird(bird)}
                                            className="flex justify-items-center w-[200px] h-[220px]"
                                        >
                                            <BlueSquare className="flex flex-col items-center h-full w-full">
                                                <div className="relative h-full w-full">
                                                    <Image
                                                        src={`/assets/birds/${bird.id}_head.png`}
                                                        alt="sun_god"
                                                        fill
                                                        className="object-contain"
                                                    />
                                                </div>

                                                <h1 className="font-bold text-[42px] whitespace-nowrap">
                                                    {bird.name}
                                                </h1>

                                            </BlueSquare>

                                        </button>
                                   
                                ))}
                            </div>
                        </div>

                        {/* Specific BIRD */}
                        <div className="flex h-full min-h-0 flex-col items-center overflow-hidden pl-3">
                            <p className="font-bold mb-10 whitespace-nowrap text-[42px]">
                                Bird Details
                            </p>

                            <BlueSquare className="flex flex-col items-center h-54 w-50">
                                <div className="relative h-full w-full">
                                    <Image
                                        src={`/assets/birds/${selectedBird.id}_head.png`}
                                        alt={`${selectedBird.id}`}
                                        fill
                                        className="object-contain"
                                    />
                                </div>

                                <h1 className="font-bold text-[42px] whitespace-nowrap">
                                    {selectedBird.name}
                                </h1>
                            </BlueSquare>


                            <div className="mt-10 flex min-h-0 w-full flex-1 flex-col gap-2 overflow-hidden">
                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Damage:</span>
                                    <span> {selectedBird.stats.damage} </span>
                                </div>


                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Attack Speed:</span>
                                    <span> {selectedBird.stats.attack_speed} </span>
                                </div>

                                <div className="min-h-0 overflow-y-auto break-words pr-2 pt-4 text-[32px] leading-relaxed">
                                   {selectedBird.desc}
                                </div>

                            </div>

                        </div>
                    </div>
                </div>


            </OrangeSquare>
        </div>
    )
}