// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import Head from "@docusaurus/Head";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

import "./DocsVersionBanner.css";

function formatBuildDate(value) {
  if (!value || value === "unknown") {
    return "unknown build";
  }
  return value.length >= 10 ? value.slice(0, 10) : value;
}

export default function DocsVersionBanner() {
  const { siteConfig } = useDocusaurusContext();
  const {
    routerVersion = "dev",
    routerBuildDate = "unknown",
    routerLatestVersion = routerVersion,
  } = siteConfig.customFields || {};
  const isDev = routerVersion === "dev";
  const buildDate = formatBuildDate(routerBuildDate);
  const latest = routerLatestVersion || routerVersion;

  return (
    <>
      <Head>
        <meta name="docs-version" content={routerVersion} />
        <meta name="docs-build-date" content={routerBuildDate} />
      </Head>
      <aside
        className={`docsVersionBanner${isDev ? " docsVersionBanner--dev" : ""}`}
        aria-label={`Docs for router ${routerVersion}, built ${routerBuildDate}`}
      >
        <span className="docsVersionBanner__label">
          {isDev ? "Development docs" : "Docs"}
        </span>
        <span className="docsVersionBanner__text">
          {isDev
            ? `Local development docs, built ${buildDate}.`
            : `Docs for router ${routerVersion} (built ${buildDate}). Latest: ${latest}.`}
        </span>
      </aside>
    </>
  );
}
