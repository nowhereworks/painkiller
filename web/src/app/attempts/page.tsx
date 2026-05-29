"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export default function AttemptPage() {
  const router = useRouter();
  const [attemptID, setAttemptID] = useState<string | null>(null);

  useEffect(() => {
    setAttemptID(new URLSearchParams(window.location.search).get("id"));
  }, []);

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-1 items-center">
      <Card className="w-full bg-card/90">
        <CardHeader>
          <CardTitle>Attempt requested</CardTitle>
          <CardDescription>The provisioning and terminal UI will be connected in the next frontend slice.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          {attemptID ? <p className="break-all text-sm text-muted-foreground">Attempt ID: {attemptID}</p> : null}
          <div className="flex gap-3">
            <Button onClick={() => router.push("/dashboard/")}>Back to dashboard</Button>
            <Button variant="outline" onClick={() => window.location.reload()}>Refresh</Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
