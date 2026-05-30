import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { Score } from "@/lib/api";

const PASS_THRESHOLD = 66;

export function ScoreCard({ score }: { score: Score }) {
  const passed = score.percentage >= PASS_THRESHOLD;

  return (
    <Card className="bg-card/90">
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <CardTitle>Score Report</CardTitle>
        <Badge className={passed ? "border-primary text-primary" : "border-destructive text-destructive"}>
          {passed ? "Passed" : "Failed"}
        </Badge>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="flex items-baseline gap-2">
          <span className="text-5xl font-bold tabular-nums">{score.percentage}%</span>
          <span className="text-muted-foreground">
            ({score.total_score} / {score.max_score} points)
          </span>
        </div>
        <div className="h-3 w-full overflow-hidden rounded-full bg-muted">
          <div
            className={`h-full rounded-full transition-all ${passed ? "bg-primary" : "bg-destructive"}`}
            style={{ width: `${score.percentage}%` }}
          />
        </div>
        <p className="text-sm text-muted-foreground">
          Passing threshold: {PASS_THRESHOLD}%
        </p>
      </CardContent>
    </Card>
  );
}
