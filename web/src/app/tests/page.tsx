"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Clock, Layers, Repeat2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { acquireFreeTest, createCheckout, listTests, type CatalogTest } from "@/lib/api";

export default function TestDetailPage() {
  const router = useRouter();
  const [test, setTest] = useState<CatalogTest | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isCheckingOut, setIsCheckingOut] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const testID = new URLSearchParams(window.location.search).get("id");

    async function loadTest() {
      try {
        const catalog = await listTests();
        const selected = catalog.tests.find((item) => item.id === testID) ?? null;
        if (!cancelled) {
          setTest(selected);
          if (!selected) {
            setError("Test not found");
          }
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Could not load test");
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    loadTest();

    return () => {
      cancelled = true;
    };
  }, []);

  async function checkout() {
    if (!test) {
      return;
    }

    setError(null);
    setIsCheckingOut(true);

    try {
      if (test.is_free) {
        await acquireFreeTest(test.id);
        router.push("/dashboard/");
      } else {
        const response = await createCheckout(test.id);
        window.location.href = response.url;
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Could not start checkout";
      setError(message);
      if (message.toLowerCase().includes("unauthorized")) {
        router.push("/login/");
      }
    } finally {
      setIsCheckingOut(false);
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-1 items-center">
      <Card className="w-full bg-card/90">
        <CardHeader>
          <CardTitle>{isLoading ? "Loading test..." : test?.title || "Test unavailable"}</CardTitle>
          <CardDescription>{test?.description || "Timed Kubernetes simulator with isolated infrastructure."}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-6">
          {test ? (
            <div className="grid gap-3 text-sm text-muted-foreground sm:grid-cols-3">
              <div className="flex items-center gap-2"><Clock className="size-4" aria-hidden="true" /> {test.duration_minutes} minutes</div>
              <div className="flex items-center gap-2"><Layers className="size-4" aria-hidden="true" /> {test.access_window_hours} hour window</div>
              <div className="flex items-center gap-2"><Repeat2 className="size-4" aria-hidden="true" /> {test.attempts_allowed} attempts</div>
            </div>
          ) : null}

          {error ? <p className="text-sm text-destructive">{error}</p> : null}

          <div className="flex flex-col gap-3 sm:flex-row">
            <Button disabled={!test || isCheckingOut} onClick={checkout}>
              {isCheckingOut ? (test?.is_free ? "Acquiring..." : "Redirecting...") : (test?.is_free ? "Get free access" : "Buy access")}
            </Button>
            <Button variant="outline" onClick={() => router.push("/")}>Back to catalog</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
