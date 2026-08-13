// Ported from my-task (origin/main @ fdceb8befbcebfa25d2216d71264bb7c5e8c96d7,
// src/app/(app)/layout.tsx) — the app shell wrapping every authenticated
// route in AuthGate/Header/Footer. Structure only, verbatim: the file
// lives at src/app/AppLayout.tsx here rather than under a "(app)" route
// group, since that's a Next.js App Router naming convention with no
// react-router equivalent — a path adaptation, not a logic/JSX change.
import { type ReactNode } from "react";
import { AuthGate } from "~/components/AuthGate";
import Header from "~/components/Header";
import Footer from "~/components/Footer";

export default function AppLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGate>
      <Header />
      {children}
      <Footer />
    </AuthGate>
  );
}
