"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { clearStoredToken, getMe, getStoredToken, logout as logoutRequest, storeToken } from "@/lib/api";

type AuthContextValue = {
  isAuthenticated: boolean;
  isAdmin: boolean;
  setAuthenticated: (token: string) => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isAdmin, setIsAdmin] = useState(false);

  useEffect(() => {
    const token = getStoredToken();
    setIsAuthenticated(Boolean(token));

    if (token) {
      getMe()
        .then((me) => setIsAdmin(me.is_admin))
        .catch(() => {
          setIsAdmin(false);
        });
    }
  }, []);

  async function setAuthenticated(token: string) {
    storeToken(token);
    setIsAuthenticated(true);

    try {
      const me = await getMe();
      setIsAdmin(me.is_admin);
    } catch {
      setIsAdmin(false);
    }
  }

  async function logout() {
    try {
      await logoutRequest();
    } catch {
      // Clearing local state is still useful if the network request fails.
    }

    clearStoredToken();
    setIsAuthenticated(false);
    setIsAdmin(false);
  }

  return <AuthContext.Provider value={{ isAuthenticated, isAdmin, setAuthenticated, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }

  return context;
}
