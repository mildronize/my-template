import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";

// fonts.css must load before globals.css: it defines the --font-manrope/
// --font-fraunces globals.css's @theme block references — real Google
// Fonts now (milestone-3/task-3, index.html's <link> tags), not task-1's
// system-font stopgap (see fonts.css's own comment).
import "~/styles/fonts.css";
import "~/styles/globals.css";

import App from "./App";
import { queryClient } from "~/lib/query-client";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("#root element not found");
}

createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
