"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { clearStoredToken, getStoredToken, logout as logoutRequest, storeToken } from "@/lib/api";

type AuthContextValue = {
  isAuthenticated: boolean;
  setAuthenticated: (token: string) => void;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    setIsAuthenticated(Boolean(getStoredToken()));
  }, []);

  function setAuthenticated(token: string) {
    storeToken(token);
    setIsAuthenticated(true);
  }

  async function logout() {
    try {
      await logoutRequest();
    } catch {
      // Clearing local state is still useful if the network request fails.
    }

    clearStoredToken();
    setIsAuthenticated(false);
  }

  return <AuthContext.Provider value={{ isAuthenticated, setAuthenticated, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }

  return context;
}
