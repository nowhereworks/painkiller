"use client";

import { useEffect, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { TestCard } from "@/components/test-card";
import { Button } from "@/components/ui/button";
import { listTests, type CatalogTest } from "@/lib/api";

export default function CatalogPage() {
  const router = useRouter();
  const [tests, setTests] = useState<CatalogTest[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [, startTransition] = useTransition();

  useEffect(() => {
    let cancelled = false;

    async function loadTests() {
      try {
        const data = await listTests();
        if (!cancelled) {
          setTests(data.tests);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "Could not load tests");
        }
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    loadTests();

    return () => {
      cancelled = true;
    };
  }, []);

  function selectTest(test: CatalogTest) {
    startTransition(() => {
      router.push(`/tests/?id=${encodeURIComponent(test.id)}`);
    });
  }

  return (
    <div className="flex flex-1 flex-col gap-10">
      <section className="max-w-3xl">
        <p className="mb-4 text-sm font-semibold uppercase tracking-[0.24em] text-primary">Kubernetes exam practice</p>
        <h1 className="text-4xl font-semibold tracking-tight sm:text-6xl">Train in real clusters, not toy terminals.</h1>
        <p className="mt-5 max-w-2xl text-lg leading-8 text-muted-foreground">
          Start a timed simulator, work from an isolated browser shell, and get final grading when the attempt ends.
        </p>
      </section>

      {error ? (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((item) => (
            <div key={item} className="h-72 animate-pulse rounded-xl border border-border bg-muted/40" />
          ))}
        </div>
      ) : tests.length > 0 ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {tests.map((test) => <TestCard key={test.id} test={test} onSelect={selectTest} />)}
        </div>
      ) : (
        <div className="rounded-xl border border-border bg-card/80 p-8 text-center">
          <h2 className="text-xl font-semibold">No tests are published yet</h2>
          <p className="mt-2 text-muted-foreground">Import a scenario version and connect it to a product to populate the catalog.</p>
          <Button className="mt-6" variant="outline" onClick={() => window.location.reload()}>Refresh catalog</Button>
        </div>
      )}
    </div>
  );
}
