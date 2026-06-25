/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  routerSidebar: [
    "overview",
    "concepts",
    {
      type: "category",
      label: "Getting Started",
      collapsed: false,
      items: [
        "getting-started/hosted-quickstart",
        "getting-started/available-models",
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
        "configuration/admin-authentication",
        "configuration/admin-authorization",
        "configuration/pii-filtering",
        "configuration/image-analysis-vlm",
        "configuration/self-hosted-upstreams",
        "configuration/model-group-contracts",
        "configuration/dynamic-score-routing",
        "configuration/routing-typescript",
        "configuration/external-routing-policy",
      ],
    },
    {
      type: "category",
      label: "Operations",
      collapsed: false,
      items: [
        "operations/key-generation",
        "operations/usage-reporting",
        "operations/admin-browser-reports",
        "operations/report-examples",
        "operations/deployment",
      ],
    },
    {
      type: "category",
      label: "Reference",
      collapsed: false,
      items: [
        "reference/api-compatibility",
        "reference/errors",
        "reference/model-metadata",
        "reference/add-provider-model",
      ],
    },
    "solution-brief",
    {
      type: "category",
      label: "Evaluate",
      collapsed: false,
      items: [
        "evaluation/evaluate-smart-router",
        "evaluation/product-capabilities",
        "evaluation/security-and-trust",
        "evaluation/cost-governance",
        "evaluation/harbor-case-study",
        "evaluation/competitive-landscape",
      ],
    },
    {
      type: "category",
      label: "Plan & Validate",
      collapsed: false,
      items: [
        "evaluation/deployment-readiness",
        "evaluation/model-group-quality",
        "evaluation/deployment-security-assessment",
        "evaluation/operational-acceptance",
      ],
    },
  ],
};

module.exports = sidebars;
