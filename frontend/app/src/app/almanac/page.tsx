"use client";

import Image from "next/image";
import { useRouter } from "next/navigation";
import OrangeSquare from "@/components/OrangeSquare";
import BlueSquare from "@/components/BlueSquare";
import ButtonWhite from "@/components/ButtonWhite";
import { ArrowLeft } from "lucide-react";

export default function AlmanacPage() {

    const route = useRouter();

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

                <h1 className="absolute left-1/2 top-12 -translate-x-1/2 text-[48px] font-bold text-amber-300">
                    SkyBloomTD Almanac
                </h1>

                <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-[100px]">
                    <div className="flex gap-[100px]">
                        <button
                            onClick={() => route.push('/almanac/birds')}
                            className="relative flex h-[300px] w-[400px] items-center"
                        >
                            <BlueSquare className="relative h-full w-full cursor-pointer">
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
                            onClick={() => route.push("/almanac/enemies")}
                            className="flex h-[300px] w-[400px] items-center"
                        >
                            <BlueSquare className="relative h-full w-full cursor-pointer">
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
            </OrangeSquare>
        </div>
    );
}