---
name: salesforce-docs-scrape
description: "Scrape Salesforce Atlas developer documentation (developer.salesforce.com/docs) to local markdown files for offline reference. Bypasses the Akamai WAF using curl-cffi (TLS fingerprint spoofing), fetches the TOC and each page from the Atlas JSON API, and converts HTML to markdown while preserving tables and code blocks and rewriting internal links. These pages are unreachable with the native WebFetch tool (the WAF returns a 403 or an empty JS shell), so use this skill whenever the target is a developer.salesforce.com/docs URL, or when the user wants to scrape, download, or mirror Salesforce developer docs / Atlas docs. Triggers: 'scrape salesforce docs', 'download SF dev docs', 'get salesforce API docs offline', or any developer.salesforce.com/docs/atlas.*.meta URL. Does NOT handle help.salesforce.com help articles."
license: MIT
---

# Salesforce Developer Docs Scraper

Scrape Salesforce Atlas **developer** documentation to local markdown, preserving
tables, code blocks, and internal links.

## Scope

- **Supported:** `developer.salesforce.com/docs/atlas.*.meta` pages (the Atlas
  framework used by all developer guides and API references).
- **Not supported:** `help.salesforce.com` help articles. Those render through a
  different, undocumented Aura RPC endpoint whose tokens rotate on every
  Salesforce release, so they are out of scope for now. If asked, say so rather
  than pointing the scraper at a help URL.

## Prerequisites

- `uv` package manager. That is the only requirement.

The scraper uses `curl-cffi`, a Python package that **bundles** the native
libcurl-impersonate binaries, so `uv run --with curl-cffi` fetches and caches
everything automatically. No separate install is needed.

## Why this skill exists

Salesforce developer docs sit behind an Akamai WAF that blocks plain
`curl`/`requests` based on their TLS fingerprint (JA3/JA4 hash) — not cookies or
headers. This means the agent's native `WebFetch` tool cannot read these pages:
it gets a 403 or an empty JavaScript shell instead of the content. This skill
closes that gap by using `curl-cffi` to spoof Chrome's fingerprint
(`impersonate="chrome"`) and fetching content straight from the Atlas JSON API,
so the docs become available offline as clean markdown.

Reach for this skill (not `WebFetch`) whenever the target is a
`developer.salesforce.com/docs` URL.

## Workflow

### 1. Identify the documentation from the URL

A developer docs URL looks like:

```text
https://developer.salesforce.com/docs/atlas.en-us.revenue_lifecycle_management_dev_guide.meta/revenue_lifecycle_management_dev_guide/product_discovery_overview.htm
```

The scraper parses this automatically. The relevant parts are:

- **Locale**: `en-us` (from `atlas.en-us.`)
- **Deliverable**: `revenue_lifecycle_management_dev_guide`
- **Root page ID**: `product_discovery_overview` (the `.htm` filename)

By default the scraper uses the root page ID from the URL as the starting TOC
node and scrapes it plus all its children. Pass `--root-id` to start from a
different node (see subsection example in the reference file).

### 2. Run the scraper

```bash
uv run --with curl-cffi --with markdownify \
  python3 skills/salesforce-docs-scrape/scripts/scraper.py \
  --url "https://developer.salesforce.com/docs/atlas.en-us.{DELIVERABLE}.meta/{DELIVERABLE}/{ROOT_PAGE}.htm" \
  --output ./docs
```

Useful flags:

- `--output, -o` — output directory (required).
- `--root-id` — override the starting TOC node (default: extracted from URL).
- `--quiet, -q` — suppress progress output.

The scraper fetches the TOC, dedupes pages (many TOC entries are `#anchor`
fragments pointing into the same page), fetches each page's content, converts
HTML to markdown, rewrites internal `.htm` links to relative `.md` paths, and
writes a `README.md` index.

### 3. Output structure

```text
docs/
├── README.md                   # Index with nested TOC links
├── {root-page}.md              # Root page
└── {root-page}/
    ├── {section}/
    │   ├── {page}.md
    │   └── {subsection}/
    │       └── {page}.md
    └── ...
```

Each file starts with a `# Title` heading and a `> Source:` link back to the
original page.

## Reference

For the underlying API endpoints, manual fetch commands (debugging), TOC-parsing
internals, subsection scraping, and troubleshooting (403s, empty content,
missing pages), see `references/api-and-troubleshooting.md`.

## Files

- `SKILL.md` — this documentation.
- `scripts/scraper.py` — the scraper (curl-cffi + markdownify).
- `references/api-and-troubleshooting.md` — API details and troubleshooting.
