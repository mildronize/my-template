// TPL-1 milestone-3: react-router wiring. Two routes, both authenticated
// and wrapped in one persistent AppLayout so AuthGate/Header/Footer don't
// remount switching between them: "/" and "/settings". There is
// deliberately no SPA route for "/login" — vite.config.ts's proxy config
// (dev) and cmd/server's wireBFF (production) both route a real browser
// navigation to /login straight to Go's own GET /login handler before it
// ever reaches react-router, so a client-side "/login" route here would be
// dead code. See vite.config.ts's proxy comment for why that's safe to
// rely on. task-1 built routing against placeholder page content; task-3
// wires the real pages underneath the same route table.
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Toaster } from "~/components/ui/sonner";
import AppLayout from "~/app/AppLayout";
import TodosPage from "~/app/TodosPage";
import SettingsPage from "~/app/settings/page";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/*"
          element={
            <AppLayout>
              <Routes>
                <Route path="/" element={<TodosPage />} />
                <Route path="/settings" element={<SettingsPage />} />
              </Routes>
            </AppLayout>
          }
        />
      </Routes>
      <Toaster />
    </BrowserRouter>
  );
}
