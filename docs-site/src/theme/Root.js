import React from "react";

import DocsVersionBanner from "../components/DocsVersionBanner";

export default function Root({ children }) {
  return (
    <>
      {children}
      <DocsVersionBanner />
    </>
  );
}
