import React from "react";

import DocsVersionBanner from "../components/DocsVersionBanner";
import SupportChatWidget from "../components/SupportChatWidget";

export default function Root({ children }) {
  return (
    <>
      {children}
      <DocsVersionBanner />
      <SupportChatWidget />
    </>
  );
}
