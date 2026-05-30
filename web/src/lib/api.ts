export type CatalogTest = {
  id: string;
  title: string;
  description: string;
  duration_minutes: number;
  access_window_hours: number;
  attempts_allowed: number;
  is_free: boolean;
};

export type AdminTest = {
  id: string;
  product_id: string;
  title: string;
  description: string;
  stripe_price_id: string | null;
  is_free: boolean;
  duration_minutes: number;
  access_window_hours: number;
  attempts_allowed: number;
};

export type MeResponse = {
  id: string;
  email: string;
  is_admin: boolean;
};

export type Purchase = {
  id: string;
  test_id: string;
  expires_at: string;
  attempts_remaining: number;
  is_active: boolean;
};

export type Attempt = {
  id: string;
  status: string;
  score?: number;
  max_score?: number;
  terminal_token?: string;
};

type ApiErrorBody = {
  error?: string;
};

const tokenKey = "painkiller-auth-token";

function apiBaseURL() {
  if (process.env.NEXT_PUBLIC_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_API_BASE_URL.replace(/\/$/, "");
  }

  return "";
}

export function getStoredToken() {
  if (typeof window === "undefined") {
    return null;
  }

  return window.localStorage.getItem(tokenKey);
}

export function storeToken(token: string) {
  window.localStorage.setItem(tokenKey, token);
}

export function clearStoredToken() {
  window.localStorage.removeItem(tokenKey);
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const token = getStoredToken();

  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${apiBaseURL()}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;
    const contentType = response.headers.get("Content-Type") ?? "";

    if (contentType.includes("application/json")) {
      const body = (await response.json()) as ApiErrorBody;
      message = body.error ?? message;
    } else {
      const body = await response.text();
      message = body || message;
    }

    throw new Error(message);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export async function listTests() {
  return apiFetch<{ tests: CatalogTest[] }>("/api/v1/catalog/tests");
}

export async function login(email: string, password: string) {
  return apiFetch<{ token: string }>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function register(email: string, password: string) {
  return apiFetch<{ id: string; email: string }>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export async function logout() {
  return apiFetch<{ status: string }>("/api/v1/auth/logout", { method: "POST" });
}

export async function getMe() {
  return apiFetch<MeResponse>("/api/v1/auth/me");
}

export async function getDashboard() {
  return apiFetch<{ purchases: Purchase[] }>("/api/v1/entitlements/dashboard");
}

export async function createCheckout(testID: string) {
  return apiFetch<{ url: string }>("/api/v1/billing/checkout", {
    method: "POST",
    body: JSON.stringify({ test_id: testID }),
  });
}

export async function acquireFreeTest(testID: string) {
  return apiFetch<{ purchase_id: string }>("/api/v1/billing/acquire-free", {
    method: "POST",
    body: JSON.stringify({ test_id: testID }),
  });
}

export async function createAttempt(purchasedTestID: string) {
  return apiFetch<Attempt>("/api/v1/attempts/", {
    method: "POST",
    body: JSON.stringify({ purchased_test_id: purchasedTestID }),
  });
}

export async function adminListTests() {
  return apiFetch<{ tests: AdminTest[] }>("/api/v1/admin/tests");
}

export async function adminGetTest(testID: string) {
  return apiFetch<AdminTest>(`/api/v1/admin/tests/${testID}`);
}

export type CreateTestRequest = {
  title: string;
  description: string;
  stripe_price_id: string | null;
  is_free: boolean;
  duration_minutes: number;
  access_window_hours: number;
  attempts_allowed: number;
};

export async function adminCreateTest(data: CreateTestRequest) {
  return apiFetch<AdminTest>("/api/v1/admin/tests", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export type UpdateTestRequest = {
  title?: string;
  description?: string;
  stripe_price_id?: string | null;
  is_free?: boolean;
  duration_minutes?: number;
  access_window_hours?: number;
  attempts_allowed?: number;
};

export async function adminUpdateTest(testID: string, data: UpdateTestRequest) {
  return apiFetch<AdminTest>(`/api/v1/admin/tests/${testID}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

export async function adminDeleteTest(testID: string) {
  return apiFetch<{ status: string }>(`/api/v1/admin/tests/${testID}`, {
    method: "DELETE",
  });
}

export async function getAttempt(attemptID: string) {
  return apiFetch<Attempt>(`/api/v1/attempts/${attemptID}`);
}

export async function submitAttempt(attemptID: string) {
  return apiFetch<{ status: string }>(`/api/v1/attempts/${attemptID}/submit`, {
    method: "POST",
  });
}

export type Score = {
  total_score: number;
  max_score: number;
  percentage: number;
  status: string;
};

export async function getScore(attemptID: string) {
  return apiFetch<Score>(`/api/v1/scoring/attempts/${attemptID}/score`);
}
