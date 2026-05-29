"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { createAttempt, type CatalogTest, type Purchase } from "@/lib/api";

export function PurchaseCard({ purchase, test }: { purchase: Purchase; test?: CatalogTest }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [isStarting, setIsStarting] = useState(false);
  const title = test?.title || "Purchased test";

  async function startAttempt() {
    setError(null);
    setIsStarting(true);

    try {
      const attempt = await createAttempt(purchase.id);
      router.push(`/attempts?id=${encodeURIComponent(attempt.id)}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start attempt");
    } finally {
      setIsStarting(false);
    }
  }

  return (
    <Card className="bg-card/90">
      <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
        <div>
          <CardTitle>{title}</CardTitle>
          <p className="mt-2 text-sm text-muted-foreground">Expires {new Date(purchase.expires_at).toLocaleString()}</p>
        </div>
        <Badge className={purchase.is_active ? "border-primary text-primary" : "border-destructive text-destructive"}>{purchase.is_active ? "Active" : "Expired"}</Badge>
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground">
        {purchase.attempts_remaining} attempts remaining
        {error ? <p className="mt-3 text-destructive">{error}</p> : null}
      </CardContent>
      <CardFooter>
        <Button disabled={!purchase.is_active || purchase.attempts_remaining < 1 || isStarting} onClick={startAttempt}>
          {isStarting ? "Starting..." : "Start attempt"}
        </Button>
      </CardFooter>
    </Card>
  );
}
