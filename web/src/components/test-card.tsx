"use client";

import { Clock, Layers, Repeat2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import type { CatalogTest } from "@/lib/api";

export function TestCard({ test, onSelect }: { test: CatalogTest; onSelect: (test: CatalogTest) => void }) {
  const title = test.title || "Kubernetes simulator";
  const description = test.description || "Timed exam practice backed by isolated kubeadm environments.";

  return (
    <Card className="flex h-full flex-col overflow-hidden bg-card/90">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm text-muted-foreground">
        <div className="flex items-center gap-2"><Clock className="size-4" aria-hidden="true" /> {test.duration_minutes} minutes</div>
        <div className="flex items-center gap-2"><Layers className="size-4" aria-hidden="true" /> {test.access_window_hours} hour access window</div>
        <div className="flex items-center gap-2"><Repeat2 className="size-4" aria-hidden="true" /> {test.attempts_allowed} attempts included</div>
      </CardContent>
      <CardFooter className="mt-auto">
        <Button className="w-full" onClick={() => onSelect(test)}>View test</Button>
      </CardFooter>
    </Card>
  );
}
