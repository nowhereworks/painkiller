"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Clock, Layers, Plus, Pencil, Trash2, Repeat2, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth";
import {
  adminListTests,
  adminCreateTest,
  adminUpdateTest,
  adminDeleteTest,
  type AdminTest,
  type CreateTestRequest,
} from "@/lib/api";

type TestFormData = {
  title: string;
  description: string;
  stripe_price_id: string;
  is_free: boolean;
  duration_minutes: number;
  access_window_hours: number;
  attempts_allowed: number;
};

const emptyForm: TestFormData = {
  title: "",
  description: "",
  stripe_price_id: "",
  is_free: false,
  duration_minutes: 120,
  access_window_hours: 36,
  attempts_allowed: 2,
};

export default function AdminPage() {
  const router = useRouter();
  const { isAuthenticated, isAdmin } = useAuth();
  const [tests, setTests] = useState<AdminTest[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editingTest, setEditingTest] = useState<AdminTest | null>(null);
  const [formData, setFormData] = useState<TestFormData>(emptyForm);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!isAuthenticated) {
      router.push("/login/");
      return;
    }
    if (!isAdmin) {
      router.push("/");
      return;
    }
    loadTests();
  }, [isAuthenticated, isAdmin, router]);

  async function loadTests() {
    try {
      const data = await adminListTests();
      setTests(data.tests);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not load tests");
    } finally {
      setIsLoading(false);
    }
  }

  function openCreateForm() {
    setEditingTest(null);
    setFormData(emptyForm);
    setShowForm(true);
  }

  function openEditForm(test: AdminTest) {
    setEditingTest(test);
    setFormData({
      title: test.title,
      description: test.description,
      stripe_price_id: test.stripe_price_id ?? "",
      is_free: test.is_free,
      duration_minutes: test.duration_minutes,
      access_window_hours: test.access_window_hours,
      attempts_allowed: test.attempts_allowed,
    });
    setShowForm(true);
  }

  function closeForm() {
    setShowForm(false);
    setEditingTest(null);
    setFormData(emptyForm);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setIsSubmitting(true);
    setError(null);

    try {
      const payload: CreateTestRequest = {
        title: formData.title,
        description: formData.description,
        stripe_price_id: formData.is_free ? null : formData.stripe_price_id || null,
        is_free: formData.is_free,
        duration_minutes: formData.duration_minutes,
        access_window_hours: formData.access_window_hours,
        attempts_allowed: formData.attempts_allowed,
      };

      if (editingTest) {
        await adminUpdateTest(editingTest.id, payload);
      } else {
        await adminCreateTest(payload);
      }

      await loadTests();
      closeForm();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save test");
    } finally {
      setIsSubmitting(false);
    }
  }

  async function handleDelete(test: AdminTest) {
    if (!confirm(`Delete "${test.title}"? This cannot be undone.`)) {
      return;
    }

    try {
      await adminDeleteTest(test.id);
      await loadTests();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not delete test");
    }
  }

  if (!isAuthenticated || !isAdmin) {
    return null;
  }

  return (
    <div className="flex flex-1 flex-col gap-8">
      <section className="flex items-center justify-between">
        <div>
          <p className="mb-3 text-sm font-semibold uppercase tracking-[0.24em] text-primary">Admin</p>
          <h1 className="text-4xl font-semibold tracking-tight">Test catalog</h1>
          <p className="mt-4 max-w-2xl text-muted-foreground">Create, edit, and delete tests in the catalog.</p>
        </div>
        <Button onClick={openCreateForm}>
          <Plus className="size-4" aria-hidden="true" />
          Create test
        </Button>
      </section>

      {error ? (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      ) : null}

      {showForm ? (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>{editingTest ? "Edit test" : "Create test"}</CardTitle>
              <Button variant="ghost" size="sm" onClick={closeForm}>
                <X className="size-4" aria-hidden="true" />
              </Button>
            </div>
            <CardDescription>
              {editingTest ? "Update the test configuration below." : "Fill in the details to create a new test."}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Label htmlFor="title">Title</Label>
                <Input
                  id="title"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                  required
                />
              </div>
              <div className="sm:col-span-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                />
              </div>
              <div className="flex items-center gap-2 sm:col-span-2">
                <input
                  type="checkbox"
                  id="is_free"
                  checked={formData.is_free}
                  onChange={(e) => setFormData({ ...formData, is_free: e.target.checked })}
                  className="size-4 rounded border-border"
                />
                <Label htmlFor="is_free">Free test (no payment required)</Label>
              </div>
              {!formData.is_free ? (
                <div className="sm:col-span-2">
                  <Label htmlFor="stripe_price_id">Stripe Price ID</Label>
                  <Input
                    id="stripe_price_id"
                    value={formData.stripe_price_id}
                    onChange={(e) => setFormData({ ...formData, stripe_price_id: e.target.value })}
                    placeholder="price_..."
                    required={!formData.is_free}
                  />
                </div>
              ) : null}
              <div>
                <Label htmlFor="duration_minutes">Duration (minutes)</Label>
                <Input
                  id="duration_minutes"
                  type="number"
                  min={1}
                  value={formData.duration_minutes}
                  onChange={(e) => setFormData({ ...formData, duration_minutes: parseInt(e.target.value) || 0 })}
                  required
                />
              </div>
              <div>
                <Label htmlFor="access_window_hours">Access window (hours)</Label>
                <Input
                  id="access_window_hours"
                  type="number"
                  min={1}
                  value={formData.access_window_hours}
                  onChange={(e) => setFormData({ ...formData, access_window_hours: parseInt(e.target.value) || 0 })}
                  required
                />
              </div>
              <div>
                <Label htmlFor="attempts_allowed">Attempts allowed</Label>
                <Input
                  id="attempts_allowed"
                  type="number"
                  min={1}
                  value={formData.attempts_allowed}
                  onChange={(e) => setFormData({ ...formData, attempts_allowed: parseInt(e.target.value) || 0 })}
                  required
                />
              </div>
              <div className="flex gap-2 sm:col-span-2">
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Saving..." : editingTest ? "Update test" : "Create test"}
                </Button>
                <Button type="button" variant="outline" onClick={closeForm}>
                  Cancel
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      ) : null}

      {isLoading ? (
        <div className="grid gap-4">
          {[0, 1, 2].map((item) => (
            <div key={item} className="h-24 animate-pulse rounded-xl border border-border bg-muted/40" />
          ))}
        </div>
      ) : tests.length > 0 ? (
        <div className="grid gap-4">
          {tests.map((test) => (
            <Card key={test.id}>
              <CardContent className="flex items-center justify-between p-6">
                <div className="flex-1">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold">{test.title}</h3>
                    {test.is_free ? (
                      <Badge className="bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100">Free</Badge>
                    ) : (
                      <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-100">Paid</Badge>
                    )}
                  </div>
                  {test.description ? (
                    <p className="mt-1 text-sm text-muted-foreground">{test.description}</p>
                  ) : null}
                  <div className="mt-2 flex flex-wrap gap-4 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Clock className="size-3" aria-hidden="true" /> {test.duration_minutes} min
                    </span>
                    <span className="flex items-center gap-1">
                      <Layers className="size-3" aria-hidden="true" /> {test.access_window_hours}h window
                    </span>
                    <span className="flex items-center gap-1">
                      <Repeat2 className="size-3" aria-hidden="true" /> {test.attempts_allowed} attempts
                    </span>
                    {!test.is_free && test.stripe_price_id ? (
                      <span className="font-mono">{test.stripe_price_id}</span>
                    ) : null}
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => openEditForm(test)}>
                    <Pencil className="size-4" aria-hidden="true" />
                  </Button>
                  <Button variant="destructive" size="sm" onClick={() => handleDelete(test)}>
                    <Trash2 className="size-4" aria-hidden="true" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <div className="rounded-xl border border-border bg-card/80 p-8 text-center">
          <h2 className="text-xl font-semibold">No tests yet</h2>
          <p className="mt-2 text-muted-foreground">Create your first test to populate the catalog.</p>
          <Button className="mt-6" onClick={openCreateForm}>Create test</Button>
        </div>
      )}
    </div>
  );
}
