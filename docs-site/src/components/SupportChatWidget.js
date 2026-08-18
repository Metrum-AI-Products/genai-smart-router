import React, { useEffect, useState } from "react";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";

/**
 * Site-wide ElevenLabs ConvAI support widget.
 * Renders only after mount so the custom element does not hydrate on the server.
 * Starts expanded with text input and transcript so voice-only orb config cannot hide chat.
 */
export default function SupportChatWidget() {
  const { siteConfig } = useDocusaurusContext();
  const agentId = siteConfig.customFields?.elevenLabsSupportAgentId;
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  if (!mounted || !agentId) {
    return null;
  }

  return React.createElement("elevenlabs-convai", {
    "agent-id": agentId,
    variant: "full",
    "default-expanded": "true",
    "always-expanded": "true",
    "text-input": "true",
    transcript: "true",
  });
}
