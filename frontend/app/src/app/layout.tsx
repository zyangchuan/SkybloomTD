import type { Metadata } from "next";
import { Concert_One } from "next/font/google";
import "./globals.css";
import PageAudioGate from "@/components/PageAudioGate";

const concert_one = Concert_One({
  weight: "400", 
  subsets: ['latin'],
  display: 'swap',
  variable: "--font-concert-one"
})

export const metadata: Metadata = {
  title: "SkybloomTD",
  description: "",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${concert_one.className} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col overflow-x-hidden">
        <PageAudioGate />
        <div className="sky-clouds" aria-hidden="true">
          {/* Top Layer */}
          <span className="cloud cloud-1 left-18 top-9" />
          <span className="cloud cloud-2 right-15 top-14" />
          <span className="cloud cloud-3 left-[40%] top-4" />
          <span className="cloud cloud-4 right-[30%] top-20" />

          {/* Upper Middle Layer */}
          <span className="cloud cloud-3 left-72 top-60" />
          <span className="cloud cloud-6 -right-4 top-48" />
          <span className="cloud cloud-7 -left-3 top-64" />
          <span className="cloud cloud-5 left-[25%] top-[25%]" />
          <span className="cloud cloud-1 right-[20%] top-[30%]" />

          {/* Lower Middle Layer */}
          <span className="cloud cloud-4 left-[5%] top-[50%]" />
          <span className="cloud cloud-2 right-[45%] top-[45%]" />
          <span className="cloud cloud-7 left-[60%] top-[55%]" />

          {/* Bottom Layer */}
          <span className="cloud cloud-6 left-12 bottom-15" />
          <span className="cloud cloud-2 right-12 bottom-20" />
          <span className="cloud cloud-1 right-80 bottom-60" />
          <span className="cloud cloud-5 left-50 bottom-60" />
          <span className="cloud cloud-3 left-[15%] bottom-[5%]" />
          <span className="cloud cloud-6 right-[35%] bottom-[10%]" />
        </div>
        {children}
      </body>
    </html>
  );
}
