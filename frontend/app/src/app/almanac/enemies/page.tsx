"use client"

import Image from "next/image";
import OrangeSquare from "@/components/OrangeSquare";
import BlueSquare from "@/components/BlueSquare";
import ButtonWhite from "@/components/ButtonWhite";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ENEMY_INFO, AlmanacEnemiesEntry}  from "@/data/almanac";

export default function EnemiesPage() {

    const route = useRouter();
    const [selectedEnemy, setSelectedEnemy] = useState< AlmanacEnemiesEntry>(ENEMY_INFO[0] ?? null);

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

                        {/* ENEMIES */}
                        <div className="border-r-4 flex flex-col min-h-0 border-orange-900/20">
                            <h1 className="mb-10 text-center font-bold text-[50px] text-zinc-300">
                                Enemies
                            </h1>

                            <div className="grid grid-cols-2 gap-10 min-h-0 justify-items-center overflow-y-auto pb-[20px]">
                                 {/* Enemies icon */}
                                {ENEMY_INFO.map((enemy) => (
                                        <button
                                            key={enemy.id}
                                            onClick={() => setSelectedEnemy(enemy)}
                                            className="flex justify-items-center w-[200px] h-[220px]"
                                        >
                                            <BlueSquare className="flex flex-col items-center h-full w-full cursor-pointer">
                                                <div className="relative h-full w-full">
                                                    <Image
                                                        src={`/assets/enemies/${enemy.id}.png`}
                                                        alt={`${enemy.id}`}
                                                        fill
                                                        className="object-contain"
                                                    />
                                                </div>

                                                <h1 className="text-[30px] whitespace-nowrap ">
                                                    {enemy.name}
                                                </h1>

                                            </BlueSquare>

                                        </button>
                                   
                                ))}
                            </div>
                        </div>

                        {/* Specific enemy */}
                        <div className="flex h-full min-h-0 flex-col items-center overflow-hidden pl-3">
                            <p className="font-bold mb-10 whitespace-nowrap text-[50px] text-zinc-300">
                                Enemy Details
                            </p>

                            <BlueSquare className="flex flex-col items-center h-54 w-50">
                                <div className="relative h-full w-full">
                                    <Image
                                        src={`/assets/enemies/${selectedEnemy.id}.png`}
                                        alt={`${selectedEnemy.id}`}
                                        fill
                                        className="object-contain"
                                    />
                                </div>

                                <h1 className="text-[30px] whitespace-nowrap">
                                    {selectedEnemy.name}
                                </h1>
                            </BlueSquare>


                            <div className="mt-10 flex min-h-0 w-full flex-1 flex-col overflow-hidden gap-1">
                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Health:</span>
                                    <span className="text-red-500"> {selectedEnemy.stats.health} </span>
                                </div>


                                <div className="border-b-4 flex items-center justify-between border-orange-900/20 pb-2">
                                    <span className="font-bold"> Movement:</span>
                                    <span className="text-red-500"> {selectedEnemy.stats.movement} </span>
                                </div>

                                <div className="min-h-0 overflow-y-auto break-words pr-2  text-[18px] leading-relaxed">
                                   {selectedEnemy.desc}
                                </div>

                            </div>

                        </div>
                    </div>
                </div>


            </OrangeSquare>
        </div>
    )
}