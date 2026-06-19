import React from "react";
import Link from "@docusaurus/Link";
import Layout from "@theme/Layout";
import useBaseUrl from "@docusaurus/useBaseUrl";
import clsx from "clsx";
import styles from "./index.module.css";

const features = [
  ["One endpoint", "OpenAI-compatible and Anthropic-compatible clients integrate once while routing policy stays server-side."],
  ["Governed access", "Caller tokens, model-group allow lists, quotas, rate limits, cache policy, and telemetry are enforced before provider calls."],
  ["Multimodal routing", "Text, image/VLM, tool-call, and coding-agent requests can share deployment-defined model groups with capability-aware target selection."],
  ["Provider optionality", "Route across validated hosted providers, OpenAI-compatible upstreams, and enterprise-owned inference services without client rewrites."],
  ["Usage intelligence", "Reports connect users, projects, model groups, providers, token counts, image events, latency, cache behavior, and request-time cost."],
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
              A governed, multi-provider gateway for LLMs, VLMs, developer tools, and AI agent workflows.
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
