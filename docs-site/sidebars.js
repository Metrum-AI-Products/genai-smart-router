/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  routerSidebar: [
    "overview",
    {
      type: "category",
      label: "Getting Started",
      collapsed: false,
      items: [
        "getting-started/hosted-quickstart",
        "getting-started/codex-cli",
        "getting-started/claude-code-cli",
      ],
    },
    {
      type: "category",
      label: "Configuration",
      collapsed: false,
      items: [
        "configuration/router-config",
        "configuration/routing-typescript",
      ],
    },
    {
      type: "category",
      label: "Operations",
      collapsed: false,
      items: [
        "operations/key-generation",
        "operations/usage-reporting",
        "operations/deployment",
      ],
    },
    "solution-brief",
    {
      type: "category",
      label: "Evaluation",
      collapsed: false,
      items: ["evaluation/harbor-case-study"],
    },
  ],
};

module.exports = sidebars;
