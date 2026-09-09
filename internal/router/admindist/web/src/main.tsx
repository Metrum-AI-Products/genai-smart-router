// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import ReactDOM from "react-dom/client";
import "@/charts/setup";
import "@/styles.css";
import App from "@/App";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
