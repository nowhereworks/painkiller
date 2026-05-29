"use client";

import Link from "next/link";
import { FormEvent, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { login } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const { setAuthenticated } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const form = new FormData(event.currentTarget);
    const email = String(form.get("email") ?? "");
    const password = String(form.get("password") ?? "");

    try {
      const response = await login(email, password);
      setAuthenticated(response.token);
      startTransition(() => router.push("/dashboard/"));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-1 items-center">
      <Card className="w-full bg-card/90">
        <CardHeader>
          <CardTitle>Log in</CardTitle>
          <CardDescription>Access your purchased Kubernetes test attempts.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={handleSubmit}>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" name="email" type="email" autoComplete="email" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <Input id="password" name="password" type="password" autoComplete="current-password" required />
            </div>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
            <Button disabled={isPending} type="submit">{isPending ? "Logging in..." : "Log in"}</Button>
          </form>
          <p className="mt-4 text-sm text-muted-foreground">No account yet? <Link className="text-primary" href="/register/">Register</Link></p>
        </CardContent>
      </Card>
    </div>
  );
}
