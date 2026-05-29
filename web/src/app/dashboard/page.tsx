"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { PurchaseCard } from "@/components/purchase-card";
import { Button } from "@/components/ui/button";
import { clearStoredToken, getDashboard, listTests, type CatalogTest, type Purchase } from "@/lib/api";

export default function DashboardPage() {
  const router = useRouter();
  const [purchases, setPurchases] = useState<Purchase[]>([]);
  const [testsByID, setTestsByID] = useState<Map<string, CatalogTest>>(new Map());
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function loadDashboard() {
      try {
        const [dashboard, catalog] = await Promise.all([getDashboard(), listTests()]);
        if (!cancelled) {
          setPurchases(dashboard.purchases);
          setTestsByID(new Map(catalog.tests.map((test) => [test.id, test])));
        }
      } catch (err) {
        if (!cancelled) {
          const message = err instanceof Error ? err.message : "Could not load dashboard";
          setError(message);
          if (message.toLowerCase().includes("unauthorized")) {
            clearStoredToken();
          }
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    loadDashboard();

    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="flex flex-1 flex-col gap-8">
      <section>
        <p className="mb-3 text-sm font-semibold uppercase tracking-[0.24em] text-primary">Student dashboard</p>
        <h1 className="text-4xl font-semibold tracking-tight">Your test access</h1>
        <p className="mt-4 max-w-2xl text-muted-foreground">Start attempts while your purchase window is active. Each start consumes one attempt.</p>
      </section>

      {error ? (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
          <Button className="ml-4" size="sm" variant="outline" onClick={() => router.push("/login/")}>Log in</Button>
        </div>
      ) : null}

      {isLoading ? (
        <div className="grid gap-4">
          {[0, 1].map((item) => <div key={item} className="h-44 animate-pulse rounded-xl border border-border bg-muted/40" />)}
        </div>
      ) : purchases.length > 0 ? (
        <div className="grid gap-4">
          {purchases.map((purchase) => <PurchaseCard key={purchase.id} purchase={purchase} test={testsByID.get(purchase.test_id)} />)}
        </div>
      ) : !error ? (
        <div className="rounded-xl border border-border bg-card/80 p-8 text-center">
          <h2 className="text-xl font-semibold">No purchases yet</h2>
          <p className="mt-2 text-muted-foreground">Pick a simulator from the catalog to begin.</p>
          <Button className="mt-6" onClick={() => router.push("/")}>Browse catalog</Button>
        </div>
      ) : null}
    </div>
  );
}
