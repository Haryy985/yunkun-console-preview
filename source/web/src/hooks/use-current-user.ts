"use client";

import { useEffect, useState } from "react";

import { auth, type CurrentUser } from "@/lib/auth";
import { api } from "@/lib/api";

const FALLBACK: CurrentUser = { id: "", name: "", username: "", email: "", avatar: "", role: "unknown" };

export function useCurrentUser(): CurrentUser {
  const [user, setUser] = useState<CurrentUser>(FALLBACK);

  useEffect(() => {
    const u = auth.getCurrentUser();
    if (u) setUser(u);
    let mounted = true;
    if (auth.getToken()) {
      api.authMe().then((identity) => {
        if (mounted) setUser({ id: String(identity.id), name: identity.username, username: identity.username, email: "", avatar: "", role: identity.role });
      }).catch(() => {});
    }
    return () => { mounted = false; };
  }, []);

  return user;
}
