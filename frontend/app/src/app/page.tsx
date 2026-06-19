import Image from 'next/image'
import OrangeSquare from '@/components/OrangeSquare';
import AuthPanel from '@/components/AuthPanel';


export default function Home() {
  return (
    // return 
    <div className="flex flex-col flex-1 items-center font-sans">
        <Image src="/skybloomtd-logo.png" alt="" width={500} height={500} className="mt-10"/>
        <OrangeSquare className="mt-10 w-full max-w-120 flex flex-col items-center justify-center">
            <AuthPanel />
        </OrangeSquare> 
    </div>
  );
}
