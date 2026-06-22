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
        "configuration/pii-filtering",
        "configuration/image-analysis-vlm",
        "configuration/self-hosted-upstreams",
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
      label: "Plan & Validate",
      collapsed: false,
      items: [
        "evaluation/product-capabilities",
        "evaluation/competitive-landscape",
        "evaluation/deployment-readiness",
        "evaluation/model-group-quality",
        "evaluation/deployment-security-assessment",
        "evaluation/operational-acceptance",
        "evaluation/cost-governance",
        "evaluation/harbor-case-study",
      ],
    },
  ],
};

module.exports = sidebars;
