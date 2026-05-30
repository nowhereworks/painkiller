"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ScoreCard } from "@/components/score-card";
import { Terminal } from "@/components/terminal";
import { getAttempt, getScore, submitAttempt, type Attempt, type Score } from "@/lib/api";

const POLL_INTERVAL = 3000;

const PROVISIONING_STATUSES = new Set([
  "purchased",
  "available",
  "attempt_requested",
  "environment_provisioning",
]);

const TERMINAL_STATUSES = new Set([
  "environment_ready",
  "terminal_opened",
  "running",
]);

const GRADING_STATUSES = new Set(["submitted", "grading"]);

export default function AttemptPage() {
  const router = useRouter();
  const [attemptID, setAttemptID] = useState<string | null>(null);
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [score, setScore] = useState<Score | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    setAttemptID(new URLSearchParams(window.location.search).get("id"));
  }, []);

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  const fetchAttempt = useCallback(async () => {
    if (!attemptID) return;
    try {
      const data = await getAttempt(attemptID);
      setAttempt(data);

      if (!PROVISIONING_STATUSES.has(data.status) && !TERMINAL_STATUSES.has(data.status)) {
        stopPolling();
      }

      if (data.status === "scored") {
        try {
          const scoreData = await getScore(attemptID);
          setScore(scoreData);
        } catch {
          // score may not be ready yet
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load attempt");
      stopPolling();
    }
  }, [attemptID, stopPolling]);

  useEffect(() => {
    if (!attemptID) return;

    fetchAttempt();

    pollRef.current = setInterval(fetchAttempt, POLL_INTERVAL);

    return () => stopPolling();
  }, [attemptID, fetchAttempt, stopPolling]);

  async function handleSubmit() {
    if (!attemptID) return;
    setIsSubmitting(true);
    try {
      await submitAttempt(attemptID);
      await fetchAttempt();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to submit");
    } finally {
      setIsSubmitting(false);
    }
  }

  if (!attemptID) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-1 items-center">
        <Card className="w-full bg-card/90">
          <CardContent className="pt-6">
            <p className="text-muted-foreground">No attempt ID provided.</p>
            <Button className="mt-4" onClick={() => router.push("/dashboard/")}>
              Back to dashboard
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (error && !attempt) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-1 items-center">
        <Card className="w-full bg-card/90">
          <CardContent className="pt-6">
            <p className="text-destructive">{error}</p>
            <Button className="mt-4" onClick={() => router.push("/dashboard/")}>
              Back to dashboard
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const status = attempt?.status ?? "loading";
  const isProvisioning = PROVISIONING_STATUSES.has(status);
  const isTerminal = TERMINAL_STATUSES.has(status);
  const isGrading = GRADING_STATUSES.has(status);
  const isScored = status === "scored";
  const isFailed = status === "provision_failed" || status === "expired" || status === "expired_before_start" || status === "expired_running";

  return (
    <div className="flex flex-1 flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="mb-1 text-sm font-semibold uppercase tracking-[0.24em] text-primary">Attempt</p>
          <h1 className="text-2xl font-semibold tracking-tight">{attemptID.slice(0, 8)}...</h1>
        </div>
        <Badge
          className={
            isTerminal
              ? "border-primary text-primary"
              : isScored
                ? "border-primary text-primary"
                : isFailed
                  ? "border-destructive text-destructive"
                  : "border-muted-foreground text-muted-foreground"
          }
        >
          {status.replace(/_/g, " ")}
        </Badge>
      </div>

      {error ? (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {isProvisioning ? (
        <Card className="bg-card/90">
          <CardHeader>
            <CardTitle>Provisioning environment</CardTitle>
            <CardDescription>
              Your Kubernetes environment is being created. This usually takes a few minutes.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-3">
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              <span className="text-sm text-muted-foreground">Please wait...</span>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {isTerminal && attempt?.terminal_token ? (
        <>
          <Terminal token={attempt.terminal_token} />
          <div className="flex justify-end gap-3">
            <Button onClick={() => router.push("/dashboard/")} variant="outline">
              Leave
            </Button>
            <Button onClick={handleSubmit} disabled={isSubmitting}>
              {isSubmitting ? "Submitting..." : "Submit attempt"}
            </Button>
          </div>
        </>
      ) : isTerminal && !attempt?.terminal_token ? (
        <Card className="bg-card/90">
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              <span className="text-sm text-muted-foreground">Waiting for terminal token...</span>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {isGrading ? (
        <Card className="bg-card/90">
          <CardHeader>
            <CardTitle>Grading in progress</CardTitle>
            <CardDescription>Your attempt is being graded. Results will appear here.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-3">
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              <span className="text-sm text-muted-foreground">Grading...</span>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {isScored && score ? <ScoreCard score={score} /> : null}

      {isFailed ? (
        <Card className="bg-card/90">
          <CardHeader>
            <CardTitle>Attempt ended</CardTitle>
            <CardDescription>
              This attempt has ended with status: {status.replace(/_/g, " ")}.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={() => router.push("/dashboard/")}>Back to dashboard</Button>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
