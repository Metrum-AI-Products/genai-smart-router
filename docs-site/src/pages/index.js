// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import clsx from "clsx";
import styles from "./index.module.css";

const features = [
  ["Stable client API", "OpenAI Chat, OpenAI Responses, Anthropic Messages, SDKs, Codex CLI, and Claude Code can call one governed endpoint."],
  ["Policy-owned model groups", "Expose quality and cost contracts instead of raw provider IDs, then tune providers and weights without client rewrites."],
  ["Request-shape safety", "Tool, image, dialect, and max-token requirements filter targets before the router calls an upstream model."],
  ["Cost and performance evidence", "Usage rows and reports preserve provider/model, tokens, image fields, cache, latency, attempts, fallbacks, and request-time cost."],
  ["Private and hosted upstreams", "Mix external providers with enterprise vLLM, SGLang, Baseten-style, or other OpenAI-compatible services behind one API."],
];

const paths = [
  ["Evaluate", "Run a complete proof path and know what evidence to request.", "/evaluation/evaluate-smart-router"],
  ["Start Coding", "Set up Codex CLI, Claude Code, or OpenAI-compatible SDK traffic.", "/getting-started/hosted-quickstart"],
  ["Plan Deployment", "Review model-group contracts, trust controls, reports, and rollout gates.", "/evaluation/deployment-readiness"],
];

export default function Home() {
  const logoUrl = useBaseUrl("/img/metrum_logo_white_new.png");
  return (
    <Layout
      title="Metrum GenAI Smart Router"
      description="Customer documentation for Metrum GenAI Smart Router"
    >
      <main className={styles.main}>
        <section className={styles.hero}>
          <div className={styles.heroInner}>
            <img src={logoUrl} alt="Metrum AI" className={styles.logo} />
            <p className={styles.eyebrow}>Metrum AI Product Documentation</p>
            <h1>GenAI Smart Router</h1>
            <p className={styles.lede}>
              A deployable enterprise gateway that keeps GenAI clients stable while routing policy, provider keys,
              model selection, budgets, telemetry, and request-time cost accounting stay under platform control.
            </p>
            <div className={styles.actions}>
              <Link className={clsx("button", styles.primary)} to="/overview">
                Product Overview
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/evaluation/evaluate-smart-router">
                Evaluate
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/solution-brief">
                Solution Brief
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/evaluation/harbor-case-study">
                Case Study
              </Link>
              <Link className={clsx("button", styles.secondary)} href="mailto:contact@metrum.ai">
                Contact Metrum
              </Link>
            </div>
          </div>
        </section>
        <section className={styles.pathBand}>
          <div className={styles.pathGrid}>
            {paths.map(([title, body, to]) => (
              <Link className={styles.path} to={to} key={title}>
                <span>{title}</span>
                <p>{body}</p>
              </Link>
            ))}
          </div>
        </section>
        <section className={styles.featureBand}>
          <div className={styles.featureGrid}>
            {features.map(([title, body]) => (
              <article className={styles.feature} key={title}>
                <h2>{title}</h2>
                <p>{body}</p>
              </article>
            ))}
          </div>
        </section>
      </main>
    </Layout>
  );
}
