// TPL-1 milestone-3: react-router wiring. Originally two routes, both
// authenticated and wrapped in one persistent AppLayout so AuthGate/
// Header/Footer don't remount switching between them: "/" and "/settings".
// There is deliberately no SPA route for "/login" — vite.config.ts's proxy
// config (dev) and cmd/server's wireBFF (production) both route a real
// browser navigation to /login straight to Go's own GET /login handler
// before it ever reaches react-router, so a client-side "/login" route
// here would be dead code. See vite.config.ts's proxy comment for why
// that's safe to rely on. task-1 built routing against placeholder page
// content; task-3 wires the real pages underneath the same route table.
//
// milestone-4/task-7 adds two more routes under the same AppLayout:
// "/todos/:id" (TodoDetailPage — GOAL.md's task-7 spec, item 1) and
// "/activity" (ActivityPage — item 3, mirrors my-task's own "/" home page
// but at its own path since this template's "/" is already TodosPage).
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Toaster } from "~/components/ui/sonner";
import AppLayout from "~/app/AppLayout";
import TodosPage from "~/app/TodosPage";
import TodoDetailPage from "~/app/todos/TodoDetailPage";
import ActivityPage from "~/app/activity/ActivityPage";
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
                <Route path="/todos/:id" element={<TodoDetailPage />} />
                <Route path="/activity" element={<ActivityPage />} />
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
