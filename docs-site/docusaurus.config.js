// @ts-check

const lightCodeTheme = require("prism-react-renderer").themes.github;
const darkCodeTheme = require("prism-react-renderer").themes.dracula;

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: "Metrum Smart LLM Router",
  tagline: "A governed, multi-provider LLM gateway for applications and coding agents.",
  favicon: "img/favicon/favicon.ico",
  url: "https://llm-api-engg.metrum.ai",
  baseUrl: "/docs/",
  organizationName: "metrum-ai",
  projectName: "smart-llmrouter",
  onBrokenLinks: "throw",
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: "warn",
    },
  },
  trailingSlash: false,

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  presets: [
    [
      "classic",
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: require.resolve("./sidebars.js"),
          routeBasePath: "/",
          editUrl: undefined,
        },
        blog: false,
        theme: {
          customCss: require.resolve("./src/css/custom.css"),
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: "img/metrum_logo_white_new.png",
      colorMode: {
        defaultMode: "dark",
        disableSwitch: false,
        respectPrefersColorScheme: false,
      },
      navbar: {
        title: "Smart LLM Router",
        logo: {
          alt: "Metrum AI",
          src: "img/metrum_logo_white_new.png",
          srcDark: "img/metrum_logo_white_new.png",
        },
        items: [
          { to: "/overview", label: "Docs", position: "left" },
          { to: "/solution-brief", label: "Solution Brief", position: "left" },
          { to: "/evaluation/harbor-case-study", label: "Case Study", position: "left" },
          { href: "mailto:contact@metrum.ai", label: "Deploy with Metrum", position: "right" },
        ],
      },
      footer: {
        style: "dark",
        links: [
          {
            title: "Product",
            items: [
              { label: "Overview", to: "/overview" },
              { label: "Configuration", to: "/configuration/router-config" },
              { label: "Operations", to: "/operations/key-generation" },
            ],
          },
          {
            title: "Evaluate",
            items: [
              { label: "Solution Brief", to: "/solution-brief" },
              { label: "Harbor Case Study", to: "/evaluation/harbor-case-study" },
              { label: "Contact Metrum", href: "mailto:contact@metrum.ai" },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Metrum AI.`,
      },
      prism: {
        theme: lightCodeTheme,
        darkTheme: darkCodeTheme,
        additionalLanguages: ["bash", "go", "typescript", "yaml"],
      },
    }),
};

module.exports = config;
