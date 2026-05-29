"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { TerminalSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/lib/auth";

export function Nav() {
  const router = useRouter();
  const { isAuthenticated, logout } = useAuth();

  async function handleLogout() {
    await logout();
    router.push("/");
  }

  return (
    <header className="border-b border-border/80 bg-background/80 backdrop-blur">
      <div className="mx-auto flex h-[72px] max-w-6xl items-center justify-between px-4 sm:px-6 lg:px-8">
        <Link href="/" className="flex items-center gap-3 font-semibold tracking-tight">
          <span className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <TerminalSquare className="size-5" aria-hidden="true" />
          </span>
          <span>Painkiller Shell</span>
        </Link>
        <nav className="flex items-center gap-2">
          <Button variant="ghost" className="hidden sm:inline-flex" onClick={() => router.push("/")}>Catalog</Button>
          {isAuthenticated ? (
            <>
              <Button variant="ghost" onClick={() => router.push("/dashboard/")}>Dashboard</Button>
              <Button variant="outline" onClick={handleLogout}>Log out</Button>
            </>
          ) : (
            <>
              <Button variant="ghost" onClick={() => router.push("/login/")}>Log in</Button>
              <Button onClick={() => router.push("/register/")}>Register</Button>
            </>
          )}
        </nav>
      </div>
    </header>
  );
}
