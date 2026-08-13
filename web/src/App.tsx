// TPL-1 milestone-3: react-router wiring. Three routes: "/" and
// "/settings" (the authenticated app, wrapped in one persistent AppLayout
// so AuthGate/Header/Footer don't remount switching between them) plus
// "/login" (deliberately outside AppLayout/AuthGate — see
// app/login/page.tsx's own comment on why a real browser navigation there
// never actually reaches this route in production, and why it's kept
// anyway). task-1 built routing against placeholder page content; task-3
// wires the real pages underneath the same route table.
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Toaster } from "~/components/ui/sonner";
import AppLayout from "~/app/AppLayout";
import TodosPage from "~/app/TodosPage";
import SettingsPage from "~/app/settings/page";
import LoginPage from "~/app/login/page";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
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
