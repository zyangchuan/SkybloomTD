"use client"

import Image from "next/image";
import OrangeSquare from "@/components/OrangeSquare";
import BlueSquare from "@/components/BlueSquare";
import ButtonWhite from "@/components/ButtonWhite";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { BIRD_INFO, AlmanacBirdsEntry } from "@/data/almanac";

export default function BirdsPage() {

    const route = useRouter();
    const [selectedBird, setSelectedBird] = useState<AlmanacBirdsEntry>(BIRD_INFO[0] ?? null);

    return (
        <div className="flex min-h-screen items-center justify-center">

            <OrangeSquare className="relative h-[800px] w-[1000px]">
                <div className="absolute translate-x-[10px]">
                    {/* Back Button */}
                    <ButtonWhite
                        onClick={() => route.back()}
                        className="flex w-[166px] h-[50px] items-center justify-center gap-2"
                    >
                        <ArrowLeft className="w-[30px] h-[60px]" />
                        <span className="text-[23px]">Back</span>
                    </ButtonWhite>
                </div>

                <div className="absolute left-[60px] top-[40px] bottom-[20px] right-[60px]">
                    {/* Main content*/}
                    <div className="grid h-full min-h-0 grid-cols-[480px_1fr]">

                        {/* BIRDS */}
                        <div className="border-r-4 flex flex-col min-h-0 border-orange-900/20">
                            <h1 className="mb-10 text-center font-bold text-[50px] text-zinc-300">
                                Birds
                            </h1>

                            <div className="grid grid-cols-2 gap-10 min-h-0 justify-items-center overflow-y-auto pb-[20px]">
                                {/* Birds icon */}
                                {BIRD_INFO.map((bird) => (
                                    <button
                                        key={bird.id}
                                        onClick={() => setSelectedBird(bird)}
                                        className="flex justify-items-center w-[200px] h-[220px]"
                                    >
                                        <BlueSquare className="flex flex-col items-center h-full w-full cursor-pointer">
                                            <div className="relative h-full w-full">
                                                <Image
                                                    src={`/assets/birds/${bird.id}_head.png`}
                                                    alt={`${bird.id}`}
                                                    fill
                                                    className="object-contain"
                                                />
                                            </div>

                                            <h1 className="text-[30px] whitespace-nowrap ">
                                                {bird.name}
                                            </h1>

                                        </BlueSquare>

                                    </button>

                                ))}
                            </div>
                        </div>

                        {/* Specific BIRD */}
                        <div className="flex h-full min-h-0 flex-col items-center overflow-hidden pl-3">
                            <p className="font-bold mb-10 whitespace-nowrap text-[50px] text-zinc-300">
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

                                <h1 className="text-[30px] whitespace-nowrap">
                                    {selectedBird.name}
                                </h1>
                            </BlueSquare>


                            <div className="mt-10 flex min-h-0 w-full flex-1 flex-col overflow-hidden gap-1">
                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Damage:</span>
                                    <span className="text-red-500"> {selectedBird.stats.damage} </span>
                                </div>


                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Attack Speed:</span>
                                    <span className="text-lime-300"> {selectedBird.stats.attack_speed}/s</span>
                                </div>

                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Attack Range:</span>
                                    <span className="text-purple-500"> {selectedBird.stats.attack_range}</span>
                                </div>

                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Attack Type:</span>
                                    <span className="text-olive-300"> {selectedBird.stats.attack_type}</span>
                                </div>

                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Cost:</span>
                                    <span className="text-yellow-300"> {selectedBird.stats.cost}</span>
                                </div>

                                {selectedBird.evolveComb && (
                                    <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                        <span className="font-bold"> Evolution Combination:</span>
                                        <span className="text-green-300"> {selectedBird.evolveComb}</span>
                                    </div>
                                )}

                                <div className="min-h-0 overflow-y-auto break-words pr-2  text-[18px] leading-relaxed">
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