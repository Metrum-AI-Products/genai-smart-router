// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import React, { useEffect, useState } from "react";
import styles from "./RouterEndpoint.module.css";

const FALLBACK_ORIGIN = "https://<router-host>";

function isLikelyEmbeddedRouterDocs() {
  if (typeof window === "undefined" || !window.location) {
    return false;
  }
  const { hostname, pathname } = window.location;
  if (!hostname || hostname === "docs.metrum.ai") {
    return false;
  }
  // Standalone Docusaurus hosts and GitHub Pages must keep the placeholder.
  // Embedded router docs are served under /docs/ from the customer router origin.
  return pathname === "/docs" || pathname.startsWith("/docs/");
}

function useRouterOrigin() {
  const [origin, setOrigin] = useState(FALLBACK_ORIGIN);

  useEffect(() => {
    if (isLikelyEmbeddedRouterDocs() && window.location?.origin) {
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
  const origin = useRouterOrigin();
  const embedded = origin !== FALLBACK_ORIGIN;
  return (
    <div className="contactBanner">
      <p>
        {embedded ? (
          <>
            These docs are built into the GenAI Smart Router server delivered for your
            deployment. Examples that show the router base URL use this browser origin, so
            on this deployment they render as <RouterOrigin /> and <RouterApiBase />.
          </>
        ) : (
          <>
            Replace <code>{FALLBACK_ORIGIN}</code> with your deployment URL. When these docs
            are served from a live router under <code>/docs/</code>, examples automatically
            use that router origin.
          </>
        )}
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
