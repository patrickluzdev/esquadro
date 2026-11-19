// The documentation site, published to GitHub Pages by .github/workflows/docs.yml.
import { readFileSync } from "node:fs";
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// .esq code blocks are colored by the VS Code extension's own grammar.
const esquadro = JSON.parse(readFileSync(new URL("../editors/vscode/syntaxes/esquadro.tmLanguage.json", import.meta.url)));

export default defineConfig({
  site: "https://patrickluzdev.github.io",
  base: "/esquadro",
  integrations: [
    starlight({
      title: "Esquadro",
      description: "Generate Go projects from a model, with blueprints.",
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/patrickluzdev/esquadro" }],
      editLink: { baseUrl: "https://github.com/patrickluzdev/esquadro/edit/main/docs/" },
      expressiveCode: {
        shiki: { langs: [{ ...esquadro, name: "esquadro", aliases: ["esq"] }] },
      },
      sidebar: [
        { label: "Start here", items: ["getting-started", "tutorial"] },
        { label: "The language", items: [{ autogenerate: { directory: "language" } }] },
        {
          label: "Blueprints",
          items: [
            "blueprints/overview",
            { label: "Reference", items: [{ autogenerate: { directory: "blueprints/reference" } }] },
            { label: "For platforms", items: [{ autogenerate: { directory: "blueprints/platform" } }] },
          ],
        },
        { label: "Guides", items: ["regenerating", "contract", "typescript-client", "frontends", "lists", "related-records", "reports", "safe-writes", "business-rules", "trash", "settings", "importing", "integrations", "audit", "notifications", "security", "email", "files", "seeds", "observability", "deploying", "production", "branching", "writing-blueprints", "editors"] },
        { label: "Reference", items: ["reference/cli", "license"] },
      ],
    }),
  ],
});
