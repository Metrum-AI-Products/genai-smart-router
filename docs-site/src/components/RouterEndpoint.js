// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import React, { useEffect, useState } from "react";
import styles from "./RouterEndpoint.module.css";

const FALLBACK_ORIGIN = "https://<router-host>";

function useRouterOrigin() {
  const [origin, setOrigin] = useState(FALLBACK_ORIGIN);

  useEffect(() => {
    if (typeof window !== "undefined" && window.location?.origin) {
      setOrigin(window.location.origin);
    }
  }, []);

  return origin;
}

export function RouterOrigin() {
  return <code>{useRouterOrigin()}</code>;
}

export function RouterApiBase() {
  return <code>{useRouterOrigin()}/v1</code>;
}

export function DeploymentSpecificNote() {
  return (
    <div className="contactBanner">
      <p>
        These docs are built into the hosted GenAI Smart Router server delivered for your deployment. Examples that
        show the router base URL use this browser origin, so on this deployment they render as <RouterOrigin /> and{" "}
        <RouterApiBase />.
      </p>
    </div>
  );
}

export function RouterCodeBlock({ children, language = "bash" }) {
  const origin = useRouterOrigin();
  const text = String(children)
    .replaceAll("{{origin}}", origin)
    .replaceAll("{{apiBase}}", `${origin}/v1`);

  return (
    <pre className={styles.codeBlock}>
      <code className={`language-${language}`}>{text.trim()}</code>
    </pre>
  );
}
