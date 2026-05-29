import type { Metadata } from "next";
import "./globals.css";
import { AuthProvider } from "@/lib/auth";
import { Nav } from "@/components/nav";

export const metadata: Metadata = {
  title: "Painkiller Shell",
  description: "Kubernetes exam practice environments",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="font-sans antialiased">
        <AuthProvider>
          <Nav />
          <main className="mx-auto flex min-h-[calc(100vh-73px)] w-full max-w-6xl flex-col px-4 py-10 sm:px-6 lg:px-8">{children}</main>
        </AuthProvider>
      </body>
    </html>
  );
}
