import React from "react";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

import "./root.css";

export default function Root({ children }) {
  const { siteConfig } = useDocusaurusContext();
  const { routerVersion, routerCommit, routerBuildDate } = siteConfig.customFields || {};
  const version = routerVersion || "dev";
  const buildDate = routerBuildDate || "unknown";

  return (
    <>
      {children}
      <div className="routerVersionBadge" aria-label={`Router version ${version}, built ${buildDate}`}>
        <span className="routerVersionBadge__label">Router</span>
        <span className="routerVersionBadge__version">{version}</span>
        <span className="routerVersionBadge__date">{buildDate}</span>
        {routerCommit && routerCommit !== version ? (
          <span className="routerVersionBadge__commit">{routerCommit}</span>
        ) : null}
      </div>
    </>
  );
}
