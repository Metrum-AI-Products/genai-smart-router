import React from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import clsx from "clsx";
import styles from "./index.module.css";

const features = [
  ["One endpoint", "OpenAI-compatible and Anthropic-compatible clients integrate once while routing policy stays server-side."],
  ["Governed access", "Caller tokens, model-group allow lists, quotas, rate limits, cache policy, and telemetry are enforced before provider calls."],
  ["Provider optionality", "Route across validated OpenRouter, MiniMax, Moonshot/Kimi, OpenAI-compatible, and Anthropic-compatible targets without client rewrites."],
  ["Measured evaluation", "The Harbor case study compares model groups with reward score, tokens, latency, throughput, cache behavior, and agent results."],
];

export default function Home() {
  const logoUrl = useBaseUrl("/img/metrum_logo_white_new.png");
  return (
    <Layout
      title="Metrum Smart LLM Router"
      description="Customer documentation for Metrum Smart LLM Router"
    >
      <main className={styles.main}>
        <section className={styles.hero}>
          <div className={styles.heroInner}>
            <img src={logoUrl} alt="Metrum AI" className={styles.logo} />
            <p className={styles.eyebrow}>Metrum AI Product Documentation</p>
            <h1>Smart LLM Router</h1>
            <p className={styles.lede}>
              A governed, multi-provider LLM gateway for applications, developer tools, and agentic coding workflows.
            </p>
            <div className={styles.actions}>
              <Link className={clsx("button", styles.primary)} to="/overview">
                Browse Docs
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/solution-brief">
                Read Solution Brief
              </Link>
              <Link className={clsx("button", styles.secondary)} to="/evaluation/harbor-case-study">
                View Case Study
              </Link>
              <Link className={clsx("button", styles.secondary)} href="mailto:contact@metrum.ai">
                Contact Metrum
              </Link>
            </div>
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
