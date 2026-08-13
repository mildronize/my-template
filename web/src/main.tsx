import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

// fonts.css must load before globals.css: it defines the --font-manrope/
// --font-fraunces fallbacks globals.css's @theme block references (see
// fonts.css's own comment for why — next/font isn't ported until task-3).
import "~/styles/fonts.css";
import "~/styles/globals.css";

import App from "./App";

const rootElement = document.getElementById("root");
if (!rootElement) {
  throw new Error("#root element not found");
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
