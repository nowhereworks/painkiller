"use client";

import Link from "next/link";
import { FormEvent, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { register } from "@/lib/api";

export default function RegisterPage() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [isPending, startTransition] = useTransition();

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const form = new FormData(event.currentTarget);
    const email = String(form.get("email") ?? "");
    const password = String(form.get("password") ?? "");

    try {
      await register(email, password);
      startTransition(() => router.push("/login/"));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Registration failed");
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-1 items-center">
      <Card className="w-full bg-card/90">
        <CardHeader>
          <CardTitle>Create account</CardTitle>
          <CardDescription>Register before buying or starting simulator attempts.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={handleSubmit}>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" name="email" type="email" autoComplete="email" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <Input id="password" name="password" type="password" autoComplete="new-password" minLength={8} required />
            </div>
            {error ? <p className="text-sm text-destructive">{error}</p> : null}
            <Button disabled={isPending} type="submit">{isPending ? "Creating..." : "Create account"}</Button>
          </form>
          <p className="mt-4 text-sm text-muted-foreground">Already registered? <Link className="text-primary" href="/login/">Log in</Link></p>
        </CardContent>
      </Card>
    </div>
  );
}
