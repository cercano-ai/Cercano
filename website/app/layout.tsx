import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? "https://cercano-ai.github.io/Cercano"),
  title: "Cercano — The free and open source AI agent for any model",
  description: "A fast, polished AI agent for frontier models, local models, and the subscriptions you already pay for.",
  openGraph: {
    title: "Cercano — The free and open source AI agent for any model",
    description: "Use frontier models, local models, and your existing subscriptions through one fast, polished agent.",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Cercano — The free and open source AI agent for any model",
    description: "Use frontier models, local models, and your existing subscriptions through one fast, polished agent.",
  },
  alternates: { canonical: "/" },
};

const themeScript = `
(() => {
  try {
    const saved = localStorage.getItem('cercano-theme');
    const preferred = matchMedia('(prefers-color-scheme: dark)').matches ? 'night' : 'day';
    document.documentElement.dataset.theme = saved || preferred;
  } catch (_) {
    document.documentElement.dataset.theme = 'day';
  }
})();`;

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
