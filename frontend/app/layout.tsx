import './globals.css';import {Shell} from '@/components/Shell';
import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/inter/600.css';
import '@fontsource/inter/700.css';
export const metadata={title:'EduCraft360 Central',description:'TechKnights Global Solutions · School customer operations',icons:{icon:'/brand-icon.png'}};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="en"><body><Shell>{children}</Shell></body></html>}
