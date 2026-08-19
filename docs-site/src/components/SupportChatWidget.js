import React, { useEffect, useRef, useState } from "react";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

const CONSENT_KEY = "metrum_docs_convai_consent";
const WIDGET_SRC =
  "/docs/vendor/elevenlabs/convai-widget-embed-0.16.3.js";

export default function SupportChatWidget() {
  const { siteConfig } = useDocusaurusContext();
  const agentId = siteConfig.customFields?.elevenLabsSupportAgentId;
  const hostRef = useRef(null);
  const [consent, setConsent] = useState(null);

  useEffect(() => {
    try {
      const saved = window.localStorage.getItem(CONSENT_KEY);
      if (saved === "granted" || saved === "denied") {
        setConsent(saved);
      }
    } catch {
      // Storage can be unavailable in hardened browsers; remain undecided.
    }
  }, []);

  useEffect(() => {
    const host = hostRef.current;
    if (!host || !agentId || consent !== "granted") {
      host?.replaceChildren();
      return undefined;
    }

    let active = true;
    let script = document.querySelector(`script[src="${WIDGET_SRC}"]`);
    if (!script) {
      script = document.createElement("script");
      script.src = WIDGET_SRC;
      script.type = "module";
      script.dataset.metrumConvai = "true";
      document.head.appendChild(script);
    }

    const mountWidget = () => {
      if (!active || consent !== "granted") {
        return;
      }
      const el = document.createElement("elevenlabs-convai");
      el.setAttribute("agent-id", agentId);
      el.setAttribute("variant", "full");
      el.setAttribute("default-expanded", "true");
      el.setAttribute("always-expanded", "true");
      el.setAttribute("text-input", "true");
      el.setAttribute("transcript", "true");
      host.replaceChildren(el);
    };
    if (customElements.get("elevenlabs-convai")) {
      mountWidget();
    } else {
      script.addEventListener("load", mountWidget, { once: true });
    }

    return () => {
      active = false;
      script.removeEventListener("load", mountWidget);
      host.replaceChildren();
      if (script.dataset.metrumConvai === "true") {
        script.remove();
      }
    };
  }, [agentId, consent]);

  const choose = (value) => {
    try {
      window.localStorage.setItem(CONSENT_KEY, value);
    } catch {
      // The in-memory choice still controls this page.
    }
    setConsent(value);
  };

  const withdraw = () => {
    try {
      window.localStorage.setItem(CONSENT_KEY, "denied");
    } catch {
      // The immediate teardown still applies to this page.
    }
    hostRef.current?.replaceChildren();
    document.querySelector(`script[src="${WIDGET_SRC}"]`)?.remove();
    setConsent("denied");
    // Unload the already-evaluated module and restore the denied preference.
    window.location.reload();
  };

  if (!agentId) {
    return null;
  }

  return (
    <>
      <aside
        aria-label="Support chat privacy controls"
        style={{
          position: "fixed",
          left: "1rem",
          bottom: "1rem",
          zIndex: 1000,
          maxWidth: "28rem",
          padding: "0.75rem",
          borderRadius: "0.5rem",
          background: "var(--ifm-background-surface-color)",
          border: "1px solid var(--ifm-color-emphasis-300)",
          boxShadow: "0 4px 14px rgba(0,0,0,.25)",
        }}
      >
        <div>
          {consent === null
            ? "Optional support chat uses ElevenLabs only after you allow it. "
            : consent === "granted"
              ? "ElevenLabs support chat is allowed. "
              : "ElevenLabs support chat is off. "}
          <a href="/docs/privacy">Privacy notice</a>
        </div>
        <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem" }}>
          {consent !== "granted" ? (
            <button type="button" onClick={() => choose("granted")}>
              Allow support chat
            </button>
          ) : (
            <button type="button" onClick={withdraw}>
              Withdraw support chat
            </button>
          )}
          {consent === null && (
            <button type="button" onClick={() => choose("denied")}>
              Decline
            </button>
          )}
        </div>
      </aside>
      <div ref={hostRef} />
    </>
  );
}
