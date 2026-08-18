import React, { useEffect, useRef } from "react";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

/**
 * Site-wide ElevenLabs ConvAI support widget.
 * Mounts a native custom element after hydration so attributes are set on the
 * real DOM node. Starts expanded with transcript plus text input.
 */
export default function SupportChatWidget() {
  const { siteConfig } = useDocusaurusContext();
  const agentId = siteConfig.customFields?.elevenLabsSupportAgentId;
  const hostRef = useRef(null);

  useEffect(() => {
    const host = hostRef.current;
    if (!host || !agentId) {
      return undefined;
    }

    const el = document.createElement("elevenlabs-convai");
    el.setAttribute("agent-id", agentId);
    el.setAttribute("variant", "full");
    el.setAttribute("default-expanded", "true");
    el.setAttribute("always-expanded", "true");
    el.setAttribute("text-input", "true");
    el.setAttribute("transcript", "true");
    host.replaceChildren(el);

    return () => {
      host.replaceChildren();
    };
  }, [agentId]);

  if (!agentId) {
    return null;
  }

  return React.createElement("div", { ref: hostRef });
}
