"use client";
import { usePathname } from "next/navigation";
import PageAudio from "./PageAudio";

// hide the setting button from quiz-overlay and mistakes-summary
const HIDDEN = ["/quiz-overlay", "/mistakes-summary"];

export default function PageAudioGate() {
    const pathname = usePathname();

    if (HIDDEN.includes(pathname)) return null;

    return <PageAudio />;
}
