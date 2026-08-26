import React, { useEffect, useRef, useState } from "react";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import "./SupportChatWidget.css";

const CONSENT_KEY = "metrum_docs_convai_consent";
const WIDGET_SRC =
  "/docs/vendor/elevenlabs/convai-widget-embed-0.16.3.js";
const PANEL_ID = "metrum-support-chat-panel";

export default function SupportChatWidget() {
  const { siteConfig } = useDocusaurusContext();
  const agentId = siteConfig.customFields?.elevenLabsSupportAgentId;
  const hostRef = useRef(null);
  const mountedRef = useRef(false);
  const [consent, setConsent] = useState(null);
  const [open, setOpen] = useState(false);
  const [showConsent, setShowConsent] = useState(false);
  const [showWithdraw, setShowWithdraw] = useState(false);

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
    if (!host) {
      return undefined;
    }
    host.hidden = !open;

    if (!agentId || consent !== "granted" || !open || mountedRef.current) {
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
      if (!active || consent !== "granted" || mountedRef.current) {
        return;
      }
      const el = document.createElement("elevenlabs-convai");
      el.setAttribute("agent-id", agentId);
      el.setAttribute("variant", "full");
      el.setAttribute("always-expanded", "true");
      el.setAttribute("default-expanded", "true");
      el.setAttribute("dismissible", "false");
      el.setAttribute("text-input", "true");
      el.setAttribute("transcript", "true");
      host.replaceChildren(el);
      mountedRef.current = true;
    };
    if (customElements.get("elevenlabs-convai")) {
      mountWidget();
    } else {
      script.addEventListener("load", mountWidget, { once: true });
    }

    return () => {
      active = false;
      script.removeEventListener("load", mountWidget);
    };
  }, [agentId, consent, open]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const onKeyDown = (event) => {
      if (event.key === "Escape") {
        setOpen(false);
        setShowWithdraw(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open]);

  const allow = () => {
    try {
      window.localStorage.setItem(CONSENT_KEY, "granted");
    } catch {
      // The in-memory choice still controls this page.
    }
    setConsent("granted");
    setShowConsent(false);
    setOpen(true);
  };

  const notNow = () => {
    setShowConsent(false);
  };

  const onLauncherClick = () => {
    setShowWithdraw(false);
    if (open) {
      setShowConsent(false);
      setOpen(false);
      return;
    }
    if (consent === "granted") {
      setShowConsent(false);
      setOpen(true);
      return;
    }
    setShowConsent(true);
  };

  const withdraw = () => {
    try {
      window.localStorage.setItem(CONSENT_KEY, "denied");
    } catch {
      // The immediate teardown still applies to this page.
    }
    mountedRef.current = false;
    hostRef.current?.replaceChildren();
    document.querySelector(`script[src="${WIDGET_SRC}"]`)?.remove();
    setConsent("denied");
    setOpen(false);
    setShowWithdraw(false);
    setShowConsent(false);
    // Unload the already-evaluated module and restore the denied preference.
    window.location.reload();
  };

  if (!agentId) {
    return null;
  }

  return (
    <>
      <div className="supportChatLauncher">
        <div className="supportChatLauncher__controls">
          <button
            type="button"
            className="supportChatLauncher__btn"
            aria-expanded={open || showConsent || showWithdraw}
            aria-controls={PANEL_ID}
            onClick={onLauncherClick}
          >
            Support
          </button>
          {consent === "granted" && (
            <button
              type="button"
              className="supportChatLauncher__more"
              aria-label="Support chat options"
              aria-expanded={showWithdraw ? "true" : "false"}
              onClick={() => {
                setShowConsent(false);
                setShowWithdraw((value) => !value);
              }}
            >
              ···
            </button>
          )}
        </div>
        {showConsent && (
          <div
            className="supportChatPopover"
            role="dialog"
            aria-label="Support chat consent"
          >
            <p className="supportChatPopover__text">
              Optional support chat stays off until you allow it. See the{" "}
              <a href="/docs/privacy">privacy notice</a>.
            </p>
            <div className="supportChatPopover__actions">
              <button
                type="button"
                className="supportChatPopover__primary"
                onClick={allow}
              >
                Allow support chat
              </button>
              <button
                type="button"
                className="supportChatPopover__secondary"
                onClick={notNow}
              >
                Not now
              </button>
            </div>
          </div>
        )}
        {showWithdraw && consent === "granted" && (
          <div
            className="supportChatPopover"
            role="dialog"
            aria-label="Support chat options"
          >
            <p className="supportChatPopover__text">
              Support chat is allowed on this browser. You can withdraw
              consent anytime.
            </p>
            <div className="supportChatPopover__actions">
              <button
                type="button"
                className="supportChatPopover__danger"
                onClick={withdraw}
              >
                Withdraw support chat
              </button>
            </div>
          </div>
        )}
      </div>
      <div
        id={PANEL_ID}
        ref={hostRef}
        className="supportChatHost"
        hidden={!open}
      />
    </>
  );
}
