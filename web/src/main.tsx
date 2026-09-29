import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";

import { App } from "./App";
import { loadSession } from "./lib/session";

// design.md 3.2 and 4.1: the token layer has to be in the document before the
// components that consume its custom properties, so the import order here is
// load-bearing rather than incidental.
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/components.css";
import "./styles/shell.css";
import "./styles/apps.css";
// The shell frame last, so it wins by import order rather than by specificity.
import "./styles/frame.css";

// The session is resolved before the first paint of any route, so a page never
// renders as "signed out" and then corrects itself a moment later. That flash is
// exactly the kind of thing 12 calls out: a state change the user did not ask
// for, arriving after the content they were reading.
void loadSession();

const container = document.getElementById("root");
if (!container) throw new Error("the #root element is missing from index.html");

createRoot(container).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
