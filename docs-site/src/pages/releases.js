// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

export default function ReleasesPage() {
  const { siteConfig } = useDocusaurusContext();
  const {
    routerVersion = "dev",
    routerBuildDate = "unknown",
    routerLatestVersion = "",
  } = siteConfig.customFields || {};

  return (
    <Layout
      title="Releases"
      description="Router release notes and bundled docs versions."
    >
      <main className="container margin-vert--lg">
        <h1>Releases</h1>
        <p>
          Each packaged router build includes browser docs for that build. Use
          this page to confirm the documented router version and open the
          release notes before an upgrade.
        </p>
        <table>
          <thead>
            <tr>
              <th>Release</th>
              <th>Build timestamp</th>
              <th>Docs</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>{routerVersion}</td>
              <td>{routerBuildDate}</td>
              <td>
                <Link to="/release-notes/">Release Notes</Link>
              </td>
            </tr>
          </tbody>
        </table>
        {routerLatestVersion && routerLatestVersion !== routerVersion ? (
          <p>Latest tagged release: {routerLatestVersion}</p>
        ) : null}
      </main>
    </Layout>
  );
}
