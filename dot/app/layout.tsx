import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Click Dot — Local Alpha",
  description: "Connect your own ChatGPT account to Click Dot on this device.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
