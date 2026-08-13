// TPL-1 milestone-3/task-1: react-router wiring, new this milestone (my-task
// used Next's file-based router). Two real routes, both behind the ported
// AppLayout shell (AuthGate/Header/Footer) — placeholder content for now,
// proving routing/embedding works end to end per this task's scope; real
// page content (todos CRUD, the settings screen's real data) is task-3's.
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Toaster } from "~/components/ui/sonner";
import AppLayout from "~/app/AppLayout";
import TodosPage from "~/app/TodosPage";
import SettingsPage from "~/app/settings/page";

export default function App() {
  return (
    <BrowserRouter>
      <AppLayout>
        <Routes>
          <Route path="/" element={<TodosPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </AppLayout>
      <Toaster />
    </BrowserRouter>
  );
}
