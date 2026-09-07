// Copyright 2006 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import React, { useEffect, useState } from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import clsx from "clsx";
import styles from "./index.module.css";

const docsLinks = [
  ["API Quickstart", "/getting-started/hosted-quickstart"],
  ["Installation", "/installation/"],
  ["Available Models And Access", "/getting-started/available-models"],
  ["Error Reference", "/reference/errors"],
  ["Routing Strategy Decision Tree", "/routing/strategy-decision-tree"],
];

export default function NotFound() {
  const logoUrl = useBaseUrl("/img/metrum_logo_white_new.png");
  const docsBaseUrl = useBaseUrl("/");
  const [docsSearchScope, setDocsSearchScope] = useState(docsBaseUrl);

  useEffect(() => {
    if (typeof window !== "undefined" && window.location?.origin) {
      setDocsSearchScope(`${window.location.origin}${docsBaseUrl}`);
    }
  }, [docsBaseUrl]);

  return (
    <Layout title="Page Not Found" description="GenAI Smart Router documentation page not found">
      <main className={styles.main}>
        <section className={clsx(styles.hero, styles.notFoundHero)}>
          <div className={styles.heroInner}>
            <img src={logoUrl} alt="Metrum AI" className={styles.logo} />
            <p className={styles.eyebrow}>Documentation</p>
            <h1>Page not found</h1>
            <p className={styles.lede}>
              The page may have moved during a documentation update. Search the docs, start from a common page,
              or report the missing link to Metrum.
            </p>
            <form className={styles.searchForm} action="https://www.google.com/search" method="get">
              <input type="hidden" name="sitesearch" value={docsSearchScope} />
              <input
                aria-label="Search GenAI Smart Router docs"
                name="q"
                type="search"
                placeholder="Search docs"
                className={styles.searchInput}
              />
              <button className={clsx("button", styles.primary)} type="submit">
                Search
              </button>
            </form>
            <div className={styles.actions}>
              <Link className={clsx("button", styles.secondary)} href="mailto:contact@metrum.ai?subject=GenAI%20Smart%20Router%20docs%20404">
                Was this useful?
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/overview">
                Docs Overview
              </Link>
            </div>
          </div>
        </section>
        <section className={styles.pathBand}>
          <div className={styles.pathGrid}>
            {docsLinks.map(([title, to]) => (
              <Link className={styles.path} to={to} key={title}>
                <span>{title}</span>
                <p>Open this commonly used documentation page.</p>
              </Link>
            ))}
          </div>
        </section>
      </main>
    </Layout>
  );
}
