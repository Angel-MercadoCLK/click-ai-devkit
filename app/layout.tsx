import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Click Dot — Local Alpha",
  description: "A local-only preview of Click Dot. No AI provider is connected.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
